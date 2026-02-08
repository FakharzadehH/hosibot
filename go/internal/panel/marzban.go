package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hosibot/internal/pkg/httpclient"
)

// MarzbanClient implements PanelClient for Marzban panels.
type MarzbanClient struct {
	baseURL   string
	username  string
	password  string
	token     string
	client    *httpclient.Client
	tokenTime time.Time
}

// NewMarzbanClient creates a new Marzban panel client.
func NewMarzbanClient(baseURL, username, password string) *MarzbanClient {
	return &MarzbanClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   httpclient.New().WithTimeout(30 * time.Second).WithInsecureSkipVerify(),
	}
}

func (m *MarzbanClient) PanelType() string {
	return "marzban"
}

// Authenticate obtains a bearer token from the Marzban panel.
func (m *MarzbanClient) Authenticate(ctx context.Context) error {
	resp, err := m.client.PostForm(m.baseURL+"/api/admin/token", map[string]string{
		"username": m.username,
		"password": m.password,
	})
	if err != nil {
		return fmt.Errorf("marzban auth failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("marzban auth parse error: %w", err)
	}

	token, ok := result["access_token"].(string)
	if !ok {
		return fmt.Errorf("marzban auth: no access_token in response")
	}

	m.token = token
	m.tokenTime = time.Now()
	m.client = m.client.WithBearerToken(token)
	return nil
}

// ensureAuth checks if token is valid and re-authenticates if needed.
func (m *MarzbanClient) ensureAuth(ctx context.Context) error {
	if m.token == "" || time.Since(m.tokenTime) > 50*time.Minute {
		return m.Authenticate(ctx)
	}
	return nil
}

func (m *MarzbanClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	resp, err := m.client.Get(m.baseURL + "/api/user/" + username)
	if err != nil {
		return nil, fmt.Errorf("marzban get user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("marzban parse error: %w", err)
	}

	user := &PanelUser{
		Username: getString(raw, "username"),
		Status:   getString(raw, "status"),
	}

	if v, ok := raw["data_limit"].(float64); ok {
		user.DataLimit = int64(v)
	}
	if v, ok := raw["used_traffic"].(float64); ok {
		user.UsedTraffic = int64(v)
	}
	if v, ok := raw["expire"].(float64); ok {
		user.ExpireTime = int64(v)
	}
	if v, ok := raw["online_at"].(string); ok {
		user.OnlineAt = 0 // parse as needed
		_ = v
	}
	if v, ok := raw["subscription_url"].(string); ok {
		user.SubLink = v
	}
	if links, ok := raw["links"].([]interface{}); ok {
		for _, l := range links {
			if s, ok := l.(string); ok {
				user.Links = append(user.Links, s)
			}
		}
	}

	return user, nil
}

// GetUserTemplate returns inbounds/proxy template extracted from an existing panel user.
// Used by admin API set_inbounds parity behavior.
func (m *MarzbanClient) GetUserTemplate(ctx context.Context, username string) (map[string][]string, map[string]string, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, nil, err
	}

	resp, err := m.client.Get(m.baseURL + "/api/user/" + username)
	if err != nil {
		return nil, nil, fmt.Errorf("marzban get user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, nil, fmt.Errorf("marzban parse error: %w", err)
	}

	if detail := strings.TrimSpace(getString(raw, "detail")); strings.EqualFold(detail, "User not found") {
		return nil, nil, fmt.Errorf("user not found")
	}

	inbounds := parseMarzbanInbounds(raw)
	proxies := parseMarzbanProxies(raw)
	if len(inbounds) == 0 && len(proxies) == 0 {
		return nil, nil, fmt.Errorf("user template not found")
	}

	// Ensure inbounds contains protocols present in proxies.
	for proto := range proxies {
		if _, ok := inbounds[proto]; !ok {
			inbounds[proto] = []string{}
		}
	}

	return inbounds, proxies, nil
}

func (m *MarzbanClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	expireTime := time.Now().Add(time.Duration(req.ExpireDays) * 24 * time.Hour).Unix()

	body := map[string]interface{}{
		"username":   req.Username,
		"status":     "active",
		"data_limit": req.DataLimit,
		"expire":     expireTime,
		"note":       req.Note,
	}

	if req.DataLimitReset != "" {
		body["data_limit_reset_strategy"] = req.DataLimitReset
	}

	if req.Inbounds != nil {
		body["inbounds"] = req.Inbounds
	}

	if req.Proxies != nil {
		proxies := make(map[string]interface{})
		for k, v := range req.Proxies {
			proxyObj := map[string]interface{}{}
			if v != "" {
				proxyObj["flow"] = v
			}
			proxies[k] = proxyObj
		}
		body["proxies"] = proxies
	}

	bodyJSON, _ := json.Marshal(body)
	resp, err := m.client.Post(m.baseURL+"/api/user", bodyJSON)
	if err != nil {
		return nil, fmt.Errorf("marzban create user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("marzban parse create response: %w", err)
	}

	// Check for error detail
	if detail, ok := raw["detail"].(string); ok && detail != "" {
		return nil, fmt.Errorf("marzban create user error: %s", detail)
	}

	user := &PanelUser{
		Username:   getString(raw, "username"),
		Status:     getString(raw, "status"),
		DataLimit:  req.DataLimit,
		ExpireTime: expireTime,
	}

	if v, ok := raw["subscription_url"].(string); ok {
		user.SubLink = v
	}
	if links, ok := raw["links"].([]interface{}); ok {
		for _, l := range links {
			if s, ok := l.(string); ok {
				user.Links = append(user.Links, s)
			}
		}
	}

	return user, nil
}

func (m *MarzbanClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

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
	if req.Inbounds != nil {
		body["inbounds"] = req.Inbounds
	}

	bodyJSON, _ := json.Marshal(body)
	resp, err := m.client.Put(m.baseURL+"/api/user/"+username, bodyJSON)
	if err != nil {
		return nil, fmt.Errorf("marzban modify user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("marzban parse modify response: %w", err)
	}

	return &PanelUser{
		Username: getString(raw, "username"),
		Status:   getString(raw, "status"),
	}, nil
}

func (m *MarzbanClient) DeleteUser(ctx context.Context, username string) error {
	if err := m.ensureAuth(ctx); err != nil {
		return err
	}

	_, err := m.client.Delete(m.baseURL + "/api/user/" + username)
	return err
}

func (m *MarzbanClient) EnableUser(ctx context.Context, username string) error {
	_, err := m.ModifyUser(ctx, username, ModifyUserRequest{Status: "active"})
	return err
}

func (m *MarzbanClient) DisableUser(ctx context.Context, username string) error {
	_, err := m.ModifyUser(ctx, username, ModifyUserRequest{Status: "disabled"})
	return err
}

func (m *MarzbanClient) ResetTraffic(ctx context.Context, username string) error {
	if err := m.ensureAuth(ctx); err != nil {
		return err
	}

	_, err := m.client.Post(m.baseURL+"/api/user/"+username+"/reset", nil)
	return err
}

func (m *MarzbanClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return "", err
	}

	resp, err := m.client.Post(m.baseURL+"/api/user/"+username+"/revoke_sub", nil)
	if err != nil {
		return "", err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return "", err
	}

	return getString(raw, "subscription_url"), nil
}

func (m *MarzbanClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	resp, err := m.client.Get(m.baseURL + "/api/inbounds")
	if err != nil {
		return nil, err
	}

	var raw map[string][]map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}

	var inbounds []PanelInbound
	for protocol, items := range raw {
		for _, item := range items {
			inbound := PanelInbound{
				Protocol: protocol,
				Tag:      getString(item, "tag"),
			}
			if v, ok := item["port"].(float64); ok {
				inbound.Port = int(v)
			}
			if v, ok := item["remark"].(string); ok {
				inbound.Remark = v
			}
			inbounds = append(inbounds, inbound)
		}
	}

	return inbounds, nil
}

func (m *MarzbanClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	resp, err := m.client.Get(m.baseURL + "/api/system")
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetNodes returns Marzban nodes from /api/nodes.
func (m *MarzbanClient) GetNodes(ctx context.Context) ([]map[string]interface{}, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	resp, err := m.client.Get(m.baseURL + "/api/nodes")
	if err != nil {
		return nil, err
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(resp, &list); err == nil {
		return list, nil
	}

	// Some panel versions may wrap response.
	var wrapped map[string]interface{}
	if err := json.Unmarshal(resp, &wrapped); err != nil {
		return nil, err
	}

	rawItems, ok := wrapped["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected marzban nodes response format")
	}

	nodes := make([]map[string]interface{}, 0, len(rawItems))
	for _, item := range rawItems {
		if node, ok := item.(map[string]interface{}); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (m *MarzbanClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	user, err := m.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return user.SubLink, nil
}

// Helper to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func parseMarzbanInbounds(raw map[string]interface{}) map[string][]string {
	out := make(map[string][]string)
	val, ok := raw["inbounds"]
	if !ok {
		return out
	}

	inboundsMap, ok := val.(map[string]interface{})
	if !ok {
		return out
	}

	for proto, tagsRaw := range inboundsMap {
		proto = strings.TrimSpace(proto)
		if proto == "" {
			continue
		}
		tags := make([]string, 0)
		switch typed := tagsRaw.(type) {
		case []interface{}:
			for _, item := range typed {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					tags = append(tags, strings.TrimSpace(s))
				}
			}
		case []string:
			for _, s := range typed {
				if strings.TrimSpace(s) != "" {
					tags = append(tags, strings.TrimSpace(s))
				}
			}
		}
		out[proto] = tags
	}
	return out
}

func parseMarzbanProxies(raw map[string]interface{}) map[string]string {
	out := make(map[string]string)

	var source map[string]interface{}
	if v, ok := raw["proxy_settings"].(map[string]interface{}); ok {
		source = v
	} else if v, ok := raw["proxies"].(map[string]interface{}); ok {
		source = v
	}
	if len(source) == 0 {
		return out
	}

	for proto, settings := range source {
		proto = strings.TrimSpace(proto)
		if proto == "" {
			continue
		}
		flow := ""
		if m, ok := settings.(map[string]interface{}); ok {
			if v, ok := m["flow"].(string); ok {
				flow = strings.TrimSpace(v)
			}
		}
		out[proto] = flow
	}
	return out
}
