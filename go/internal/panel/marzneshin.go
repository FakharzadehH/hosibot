package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"hosibot/internal/pkg/httpclient"
)

// MarzneshinClient implements PanelClient for Marzneshin panels.
type MarzneshinClient struct {
	baseURL         string
	username        string
	password        string
	token           string
	tokenTime       time.Time
	defaultServices []string
	startOnFirstUse bool
	client          *httpclient.Client
}

func NewMarzneshinClient(baseURL, username, password string, defaultServices []string, startOnFirstUse bool) PanelClient {
	return &MarzneshinClient{
		baseURL:         strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username:        strings.TrimSpace(username),
		password:        password,
		defaultServices: defaultServices,
		startOnFirstUse: startOnFirstUse,
		client:          httpclient.New().WithTimeout(30 * time.Second).WithInsecureSkipVerify(),
	}
}

func (m *MarzneshinClient) PanelType() string { return "marzneshin" }

func (m *MarzneshinClient) Authenticate(ctx context.Context) error {
	form := map[string]string{
		"username": m.username,
		"password": m.password,
	}

	resp, err := m.client.PostForm(m.baseURL+"/api/admins/token", form)
	if err != nil {
		// Some deployments use Marzban-compatible path.
		resp, err = m.client.PostForm(m.baseURL+"/api/admin/token", form)
	}
	if err != nil {
		return fmt.Errorf("marzneshin auth failed: %w", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(resp, &out); err != nil {
		return fmt.Errorf("marzneshin auth parse error: %w", err)
	}
	token := strings.TrimSpace(getString(out, "access_token"))
	if token == "" {
		return fmt.Errorf("marzneshin auth: no access_token in response")
	}

	m.token = token
	m.tokenTime = time.Now()
	m.client = m.client.WithBearerToken(token)
	return nil
}

func (m *MarzneshinClient) ensureAuth(ctx context.Context) error {
	if m.token == "" || time.Since(m.tokenTime) > 50*time.Minute {
		return m.Authenticate(ctx)
	}
	return nil
}

func (m *MarzneshinClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	resp, err := m.client.Get(m.baseURL + "/api/users/" + username)
	if err != nil {
		return nil, fmt.Errorf("marzneshin get user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("marzneshin parse user failed: %w", err)
	}
	if detail := strings.TrimSpace(getString(raw, "detail")); detail != "" {
		return nil, fmt.Errorf(detail)
	}

	status := "active"
	if enabled, ok := raw["enabled"].(bool); ok && !enabled {
		status = "disabled"
	}
	if expired, ok := raw["expired"].(bool); ok && expired {
		status = "expired"
	}
	if strings.EqualFold(strings.TrimSpace(getString(raw, "expire_strategy")), "start_on_first_use") {
		status = "on_hold"
	}
	if dataLimit, used := toInt64(raw["data_limit"]), toInt64(raw["used_traffic"]); dataLimit > 0 && dataLimit-used <= 0 {
		status = "limited"
	}

	user := &PanelUser{
		Username:    strings.TrimSpace(getString(raw, "username")),
		Status:      status,
		DataLimit:   toInt64(raw["data_limit"]),
		UsedTraffic: toInt64(raw["used_traffic"]),
		SubLink:     absolutizeURL(m.baseURL, getString(raw, "subscription_url")),
		Note:        strings.TrimSpace(getString(raw, "note")),
	}
	if serviceIDs, ok := raw["service_ids"].([]interface{}); ok {
		user.Inbounds = map[string]string{}
		user.Proxies = map[string]string{}
		for _, item := range serviceIDs {
			id := strings.TrimSpace(fmt.Sprintf("%v", item))
			if id == "" {
				continue
			}
			user.Inbounds[id] = id
			user.Proxies[id] = ""
		}
	}

	if t := parseAnyTime(raw["expire_date"]); t > 0 {
		user.ExpireTime = t
	}

	// Marzneshin generally returns a text subscription payload behind URL.
	if link := strings.TrimSpace(user.SubLink); link != "" {
		user.Links = []string{link}
	}

	return user, nil
}

func (m *MarzneshinClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"username":   req.Username,
		"data_limit": req.DataLimit,
		"note":       req.Note,
	}

	serviceIDs := flattenServiceIDs(req.Inbounds)
	if len(serviceIDs) == 0 {
		serviceIDs = m.defaultServices
	}
	if len(serviceIDs) > 0 {
		payload["service_ids"] = serviceIDs
	}

	if req.DataLimitReset != "" {
		payload["data_limit_reset_strategy"] = req.DataLimitReset
	}

	if req.ExpireDays <= 0 {
		payload["expire_strategy"] = "never"
	} else if m.startOnFirstUse {
		payload["expire_strategy"] = "start_on_first_use"
		payload["usage_duration"] = req.ExpireDays * 86400
	} else {
		payload["expire_strategy"] = "fixed_date"
		payload["expire_date"] = time.Now().Add(time.Duration(req.ExpireDays) * 24 * time.Hour).Format(time.RFC3339)
	}

	body, _ := json.Marshal(payload)
	resp, err := m.client.Post(m.baseURL+"/api/users", body)
	if err != nil {
		return nil, fmt.Errorf("marzneshin create user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("marzneshin create parse failed: %w", err)
	}
	if detail := strings.TrimSpace(getString(raw, "detail")); detail != "" {
		return nil, fmt.Errorf(detail)
	}

	return m.GetUser(ctx, req.Username)
}

func (m *MarzneshinClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"username": username,
	}
	if req.DataLimit > 0 {
		payload["data_limit"] = req.DataLimit
	}
	if req.Note != "" {
		payload["note"] = req.Note
	}
	if req.DataLimitReset != "" {
		payload["data_limit_reset_strategy"] = req.DataLimitReset
	}
	if req.ExpireTime > 0 {
		payload["expire_strategy"] = "fixed_date"
		payload["expire_date"] = time.Unix(req.ExpireTime, 0).UTC().Format(time.RFC3339)
	}
	if req.Status != "" {
		switch strings.ToLower(strings.TrimSpace(req.Status)) {
		case "active":
			payload["enabled"] = true
		case "disable", "disabled":
			payload["enabled"] = false
		}
	}

	body, _ := json.Marshal(payload)
	resp, err := m.client.Put(m.baseURL+"/api/users/"+username, body)
	if err != nil {
		return nil, fmt.Errorf("marzneshin modify user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("marzneshin modify parse failed: %w", err)
	}
	if detail := strings.TrimSpace(getString(raw, "detail")); detail != "" {
		return nil, fmt.Errorf(detail)
	}

	return m.GetUser(ctx, username)
}

func (m *MarzneshinClient) DeleteUser(ctx context.Context, username string) error {
	if err := m.ensureAuth(ctx); err != nil {
		return err
	}
	_, err := m.client.Delete(m.baseURL + "/api/users/" + username)
	return err
}

func (m *MarzneshinClient) EnableUser(ctx context.Context, username string) error {
	_, err := m.ModifyUser(ctx, username, ModifyUserRequest{Status: "active"})
	return err
}

func (m *MarzneshinClient) DisableUser(ctx context.Context, username string) error {
	_, err := m.ModifyUser(ctx, username, ModifyUserRequest{Status: "disabled"})
	return err
}

func (m *MarzneshinClient) ResetTraffic(ctx context.Context, username string) error {
	if err := m.ensureAuth(ctx); err != nil {
		return err
	}
	_, err := m.client.Post(m.baseURL+"/api/users/"+username+"/reset", nil)
	return err
}

func (m *MarzneshinClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return "", err
	}
	resp, err := m.client.Post(m.baseURL+"/api/users/"+username+"/revoke_sub", nil)
	if err != nil {
		return "", err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return "", err
	}
	link := absolutizeURL(m.baseURL, getString(raw, "subscription_url"))
	if strings.TrimSpace(link) == "" {
		user, gErr := m.GetUser(ctx, username)
		if gErr != nil {
			return "", gErr
		}
		return user.SubLink, nil
	}
	return link, nil
}

func (m *MarzneshinClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	out := make([]PanelInbound, 0, len(m.defaultServices))
	for _, item := range m.defaultServices {
		tag := strings.TrimSpace(item)
		if tag == "" {
			continue
		}
		out = append(out, PanelInbound{
			Tag:      tag,
			Protocol: "service",
		})
	}
	return out, nil
}

func (m *MarzneshinClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	if err := m.ensureAuth(ctx); err != nil {
		return nil, err
	}
	resp, err := m.client.Get(m.baseURL + "/api/system/stats/users")
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (m *MarzneshinClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	user, err := m.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return user.SubLink, nil
}

func flattenServiceIDs(inbounds map[string][]string) []string {
	if len(inbounds) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(inbounds))
	for proto, tags := range inbounds {
		proto = strings.TrimSpace(proto)
		if len(tags) == 0 && proto != "" {
			if _, ok := seen[proto]; !ok {
				seen[proto] = struct{}{}
				out = append(out, proto)
			}
			continue
		}
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}

func parseAnyTime(v interface{}) int64 {
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" || s == "<nil>" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		// Already a unix timestamp in seconds or milliseconds.
		if n > 1_000_000_000_000 {
			return n / 1000
		}
		return n
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		if p2, e2 := time.Parse("2006-01-02 15:04:05", s); e2 == nil {
			return p2.Unix()
		}
		return 0
	}
	return parsed.Unix()
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return t
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

func absolutizeURL(base, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(raw, "/")
}
