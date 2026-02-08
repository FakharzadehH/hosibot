package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hosibot/internal/pkg/httpclient"
)

// PasarGuardClient implements PanelClient for PasarGuard panels.
//
// PasarGuard OpenAPI docs:
//   - https://usa.hosidesu.tech:8000/docs#/
//   - /openapi.json (OpenAPI 3.1, version 1.11.0)
//
// Supported endpoints used by the bot:
//   - POST /api/admin/token
//   - GET/POST/PUT/DELETE /api/user{/{username}}
//   - POST /api/user/{username}/reset
//   - POST /api/user/{username}/revoke_sub
//   - GET /api/inbounds
//   - GET /api/system
//
// Backward compatibility:
//   - If username/password are empty, secret token can be used directly.
type PasarGuardClient struct {
	baseURL  string
	username string
	password string
	apiToken string // static bearer token fallback (legacy secret_code)

	client    *httpclient.Client
	token     string
	tokenTime time.Time
	inbounds  []string // default protocol hints for proxy generation
}

// NewPasarGuardClient creates a new PasarGuard panel client.
func NewPasarGuardClient(baseURL, username, password, secretToken string) *PasarGuardClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	c := &PasarGuardClient{
		baseURL:  baseURL,
		username: strings.TrimSpace(username),
		password: password,
		apiToken: strings.TrimSpace(secretToken),
		client: httpclient.New().
			WithTimeout(30*time.Second).
			WithInsecureSkipVerify().
			WithHeader("Accept", "application/json"),
	}

	if c.apiToken != "" {
		c.token = c.apiToken
		c.tokenTime = time.Now()
		c.client.WithBearerToken(c.token)
	}

	return c
}

func (p *PasarGuardClient) PanelType() string {
	return "pasarguard"
}

// Authenticate logs in against /api/admin/token or validates static bearer token.
func (p *PasarGuardClient) Authenticate(ctx context.Context) error {
	if p.username != "" && p.password != "" {
		resp, status, err := p.doFormRequest(ctx, "POST", "/api/admin/token", map[string]string{
			"username": p.username,
			"password": p.password,
		})
		if err != nil {
			return fmt.Errorf("pasarguard auth failed: %w", err)
		}
		if status >= 400 {
			return fmt.Errorf("pasarguard auth failed: %s", p.extractAPIError(resp, status))
		}

		var tokenResp struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(resp, &tokenResp); err != nil {
			return fmt.Errorf("pasarguard auth parse error: %w", err)
		}
		if strings.TrimSpace(tokenResp.AccessToken) == "" {
			return fmt.Errorf("pasarguard auth failed: no access_token in response")
		}

		p.token = tokenResp.AccessToken
		p.tokenTime = time.Now()
		p.client.WithBearerToken(p.token)
		return nil
	}

	if p.apiToken == "" {
		return fmt.Errorf("pasarguard auth failed: missing credentials and secret token")
	}

	p.token = p.apiToken
	p.tokenTime = time.Now()
	p.client.WithBearerToken(p.token)

	resp, status, err := p.doAuthRequest(ctx, "GET", "/api/system", nil, nil)
	if err != nil {
		return fmt.Errorf("pasarguard auth check failed: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("pasarguard auth check failed: %s", p.extractAPIError(resp, status))
	}
	return nil
}

// ensureAuth refreshes token for credential-based auth.
func (p *PasarGuardClient) ensureAuth(ctx context.Context) error {
	if p.username != "" && p.password != "" {
		if p.token == "" || time.Since(p.tokenTime) > 50*time.Minute {
			return p.Authenticate(ctx)
		}
		return nil
	}

	if p.token == "" {
		if p.apiToken == "" {
			return fmt.Errorf("pasarguard auth failed: token is empty")
		}
		p.token = p.apiToken
		p.tokenTime = time.Now()
		p.client.WithBearerToken(p.token)
	}
	return nil
}

func (p *PasarGuardClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	resp, status, err := p.doAuthRequest(ctx, "GET", "/api/user/"+url.PathEscape(username), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pasarguard get user failed: %w", err)
	}
	if status == 404 {
		return nil, fmt.Errorf("user %s not found", username)
	}
	if status >= 400 {
		return nil, fmt.Errorf("pasarguard get user failed: %s", p.extractAPIError(resp, status))
	}

	user, err := p.parseUserResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("pasarguard parse user response: %w", err)
	}
	return user, nil
}

func (p *PasarGuardClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	expireTime := int64(0)
	if req.ExpireDays > 0 {
		expireTime = time.Now().Add(time.Duration(req.ExpireDays) * 24 * time.Hour).Unix()
	}

	body := map[string]interface{}{
		"username":                  req.Username,
		"status":                    "active",
		"data_limit":                req.DataLimit,
		"proxy_settings":            p.buildProxySettings(req),
		"data_limit_reset_strategy": req.DataLimitReset,
	}

	if req.Note != "" {
		body["note"] = req.Note
	}
	if expireTime > 0 {
		body["expire"] = expireTime
	}
	if req.DataLimitReset == "" {
		delete(body, "data_limit_reset_strategy")
	}

	resp, status, err := p.doAuthRequest(ctx, "POST", "/api/user", body, nil)
	if err != nil {
		return nil, fmt.Errorf("pasarguard create user failed: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("pasarguard create user failed: %s", p.extractAPIError(resp, status))
	}

	user, err := p.parseUserResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("pasarguard parse create response: %w", err)
	}
	return user, nil
}

func (p *PasarGuardClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	body := map[string]interface{}{}

	if req.Status != "" {
		body["status"] = req.Status
	}
	if req.DataLimit > 0 {
		body["data_limit"] = req.DataLimit
	}
	if req.ExpireTime > 0 {
		body["expire"] = req.ExpireTime
	}
	if req.Note != "" {
		body["note"] = req.Note
	}
	if req.DataLimitReset != "" {
		body["data_limit_reset_strategy"] = req.DataLimitReset
	}

	if len(body) == 0 {
		return p.GetUser(ctx, username)
	}

	resp, status, err := p.doAuthRequest(ctx, "PUT", "/api/user/"+url.PathEscape(username), body, nil)
	if err != nil {
		return nil, fmt.Errorf("pasarguard modify user failed: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("pasarguard modify user failed: %s", p.extractAPIError(resp, status))
	}

	user, err := p.parseUserResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("pasarguard parse modify response: %w", err)
	}
	return user, nil
}

func (p *PasarGuardClient) DeleteUser(ctx context.Context, username string) error {
	resp, status, err := p.doAuthRequest(ctx, "DELETE", "/api/user/"+url.PathEscape(username), nil, nil)
	if err != nil {
		return fmt.Errorf("pasarguard delete user failed: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("pasarguard delete user failed: %s", p.extractAPIError(resp, status))
	}
	return nil
}

func (p *PasarGuardClient) EnableUser(ctx context.Context, username string) error {
	_, err := p.ModifyUser(ctx, username, ModifyUserRequest{Status: "active"})
	return err
}

func (p *PasarGuardClient) DisableUser(ctx context.Context, username string) error {
	_, err := p.ModifyUser(ctx, username, ModifyUserRequest{Status: "disabled"})
	return err
}

func (p *PasarGuardClient) ResetTraffic(ctx context.Context, username string) error {
	resp, status, err := p.doAuthRequest(ctx, "POST", "/api/user/"+url.PathEscape(username)+"/reset", nil, nil)
	if err != nil {
		return fmt.Errorf("pasarguard reset traffic failed: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("pasarguard reset traffic failed: %s", p.extractAPIError(resp, status))
	}
	return nil
}

func (p *PasarGuardClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	resp, status, err := p.doAuthRequest(ctx, "POST", "/api/user/"+url.PathEscape(username)+"/revoke_sub", nil, nil)
	if err != nil {
		return "", fmt.Errorf("pasarguard revoke subscription failed: %w", err)
	}
	if status >= 400 {
		return "", fmt.Errorf("pasarguard revoke subscription failed: %s", p.extractAPIError(resp, status))
	}

	user, err := p.parseUserResponse(resp)
	if err != nil {
		return "", fmt.Errorf("pasarguard parse revoke response: %w", err)
	}
	return user.SubLink, nil
}

func (p *PasarGuardClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	resp, status, err := p.doAuthRequest(ctx, "GET", "/api/inbounds", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pasarguard get inbounds failed: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("pasarguard get inbounds failed: %s", p.extractAPIError(resp, status))
	}

	var tags []string
	if err := json.Unmarshal(resp, &tags); err == nil {
		inbounds := make([]PanelInbound, 0, len(tags))
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			inbounds = append(inbounds, PanelInbound{Tag: tag, Remark: tag})
		}
		return inbounds, nil
	}

	var grouped map[string][]map[string]interface{}
	if err := json.Unmarshal(resp, &grouped); err == nil {
		var inbounds []PanelInbound
		for protocol, items := range grouped {
			for _, item := range items {
				tag := getString(item, "tag")
				if tag == "" {
					continue
				}
				inbound := PanelInbound{Tag: tag, Protocol: protocol, Remark: getString(item, "remark")}
				if port, ok := item["port"].(float64); ok {
					inbound.Port = int(port)
				}
				if inbound.Remark == "" {
					inbound.Remark = tag
				}
				inbounds = append(inbounds, inbound)
			}
		}
		return inbounds, nil
	}

	return nil, fmt.Errorf("pasarguard inbounds parse error: unsupported response format")
}

func (p *PasarGuardClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	resp, status, err := p.doAuthRequest(ctx, "GET", "/api/system", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pasarguard system stats failed: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("pasarguard system stats failed: %s", p.extractAPIError(resp, status))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("pasarguard system stats parse error: %w", err)
	}
	return result, nil
}

func (p *PasarGuardClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	user, err := p.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	if user.SubLink == "" {
		return "", fmt.Errorf("subscription link is empty")
	}
	return user.SubLink, nil
}

// SetInbounds stores default protocol hints for create operations.
func (p *PasarGuardClient) SetInbounds(inbounds []string) {
	seen := make(map[string]bool)
	clean := make([]string, 0, len(inbounds))
	for _, item := range inbounds {
		norm := strings.ToLower(strings.TrimSpace(item))
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		clean = append(clean, norm)
	}
	p.inbounds = clean
}

// doAuthRequest sends an authenticated request and retries once on 401.
func (p *PasarGuardClient) doAuthRequest(ctx context.Context, method, path string, body interface{}, query map[string]string) ([]byte, int, error) {
	if err := p.ensureAuth(ctx); err != nil {
		return nil, 0, err
	}

	resp, status, err := p.doRequest(ctx, method, path, body, query)
	if err != nil {
		return nil, status, err
	}

	if status == 401 && p.username != "" && p.password != "" {
		if err := p.Authenticate(ctx); err != nil {
			return resp, status, err
		}
		return p.doRequest(ctx, method, path, body, query)
	}

	return resp, status, nil
}

func (p *PasarGuardClient) doRequest(ctx context.Context, method, path string, body interface{}, query map[string]string) ([]byte, int, error) {
	req := p.client.Raw().R().SetContext(ctx)
	if body != nil {
		req = req.SetHeader("Content-Type", "application/json").SetBody(body)
	}
	for k, v := range query {
		req.SetQueryParam(k, v)
	}

	endpoint := p.baseURL + path
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET":
		res, err := req.Get(endpoint)
		if err != nil {
			return nil, 0, err
		}
		return res.Body(), res.StatusCode(), nil
	case "POST":
		res, err := req.Post(endpoint)
		if err != nil {
			return nil, 0, err
		}
		return res.Body(), res.StatusCode(), nil
	case "PUT":
		res, err := req.Put(endpoint)
		if err != nil {
			return nil, 0, err
		}
		return res.Body(), res.StatusCode(), nil
	case "DELETE":
		res, err := req.Delete(endpoint)
		if err != nil {
			return nil, 0, err
		}
		return res.Body(), res.StatusCode(), nil
	default:
		return nil, 0, fmt.Errorf("unsupported HTTP method: %s", method)
	}
}

func (p *PasarGuardClient) doFormRequest(ctx context.Context, method, path string, form map[string]string) ([]byte, int, error) {
	endpoint := p.baseURL + path
	req := p.client.Raw().R().SetContext(ctx).SetFormData(form)

	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "POST":
		res, err := req.Post(endpoint)
		if err != nil {
			return nil, 0, err
		}
		return res.Body(), res.StatusCode(), nil
	default:
		return nil, 0, fmt.Errorf("unsupported form HTTP method: %s", method)
	}
}

func (p *PasarGuardClient) parseUserResponse(raw []byte) (*PanelUser, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	user := &PanelUser{
		Username:       getString(data, "username"),
		Status:         getString(data, "status"),
		UsedTraffic:    parseInt64Any(data["used_traffic"]),
		DataLimit:      parseInt64Any(data["data_limit"]),
		ExpireTime:     parseTimeToUnix(data["expire"]),
		OnlineAt:       parseTimeToUnix(data["online_at"]),
		SubLink:        p.makeAbsoluteSubURL(getString(data, "subscription_url")),
		Note:           getString(data, "note"),
		DataLimitReset: getString(data, "data_limit_reset_strategy"),
	}

	if links, ok := data["links"].([]interface{}); ok {
		for _, link := range links {
			if s, ok := link.(string); ok && strings.TrimSpace(s) != "" {
				user.Links = append(user.Links, strings.TrimSpace(s))
			}
		}
	}

	if proxySettings, ok := data["proxy_settings"].(map[string]interface{}); ok {
		user.Proxies = flattenProxySettings(proxySettings)
	}

	if user.Status == "" {
		user.Status = "active"
	}

	return user, nil
}

func (p *PasarGuardClient) buildProxySettings(req CreateUserRequest) map[string]interface{} {
	protocols := make([]string, 0)
	seen := make(map[string]bool)

	addProtocol := func(proto string) {
		proto = strings.ToLower(strings.TrimSpace(proto))
		if proto == "" || seen[proto] {
			return
		}
		switch proto {
		case "vmess", "vless", "trojan", "shadowsocks":
			seen[proto] = true
			protocols = append(protocols, proto)
		}
	}

	for proto := range req.Proxies {
		addProtocol(proto)
	}
	for proto := range req.Inbounds {
		addProtocol(proto)
	}
	for _, proto := range p.inbounds {
		addProtocol(proto)
	}
	if len(protocols) == 0 {
		addProtocol("vmess")
	}

	proxySettings := make(map[string]interface{}, len(protocols))
	for _, proto := range protocols {
		switch proto {
		case "vmess":
			proxySettings[proto] = map[string]interface{}{"id": uuid.NewString()}
		case "vless":
			flow := strings.TrimSpace(req.Proxies["vless"])
			if flow == "" {
				flow = strings.TrimSpace(req.FlowType)
			}
			item := map[string]interface{}{"id": uuid.NewString()}
			if flow != "" {
				item["flow"] = flow
			}
			proxySettings[proto] = item
		case "trojan":
			proxySettings[proto] = map[string]interface{}{"password": uuid.NewString()}
		case "shadowsocks":
			proxySettings[proto] = map[string]interface{}{
				"password": uuid.NewString(),
				"method":   "chacha20-ietf-poly1305",
			}
		}
	}

	return proxySettings
}

func (p *PasarGuardClient) makeAbsoluteSubURL(subURL string) string {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return ""
	}
	if strings.HasPrefix(subURL, "http://") || strings.HasPrefix(subURL, "https://") {
		return subURL
	}
	return p.baseURL + "/" + strings.TrimLeft(subURL, "/")
}

func (p *PasarGuardClient) extractAPIError(body []byte, status int) string {
	if len(body) == 0 {
		return fmt.Sprintf("HTTP %d", status)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Sprintf("HTTP %d", status)
	}

	if detail, ok := parsed["detail"].(string); ok && strings.TrimSpace(detail) != "" {
		return detail
	}
	if errMsg, ok := parsed["error"].(string); ok && strings.TrimSpace(errMsg) != "" {
		return errMsg
	}
	if msg, ok := parsed["msg"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}

	return fmt.Sprintf("HTTP %d", status)
}

func flattenProxySettings(proxySettings map[string]interface{}) map[string]string {
	out := make(map[string]string)
	for proto, raw := range proxySettings {
		rawObj, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := rawObj["id"].(string); ok && strings.TrimSpace(id) != "" {
			out[proto] = id
			continue
		}
		if pwd, ok := rawObj["password"].(string); ok && strings.TrimSpace(pwd) != "" {
			out[proto] = pwd
			continue
		}
		if flow, ok := rawObj["flow"].(string); ok && strings.TrimSpace(flow) != "" {
			out[proto] = flow
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseInt64Any(v interface{}) int64 {
	switch val := v.(type) {
	case nil:
		return 0
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	case string:
		if strings.TrimSpace(val) == "" {
			return 0
		}
		var out int64
		_, _ = fmt.Sscanf(strings.TrimSpace(val), "%d", &out)
		return out
	default:
		return 0
	}
}

func parseTimeToUnix(v interface{}) int64 {
	switch val := v.(type) {
	case nil:
		return 0
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return 0
		}
		var unix int64
		if _, err := fmt.Sscanf(s, "%d", &unix); err == nil {
			return unix
		}

		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, s); err == nil {
				return t.Unix()
			}
		}
	}
	return 0
}
