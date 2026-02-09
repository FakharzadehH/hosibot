package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"hosibot/internal/pkg/httpclient"
)

// HiddifyClient implements PanelClient for Hiddify panels.
type HiddifyClient struct {
	baseURL string
	apiKey  string
	linkSub string
	client  *httpclient.Client
}

func NewHiddifyClient(baseURL, apiKey, linkSub string) PanelClient {
	return &HiddifyClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		linkSub: strings.TrimSpace(linkSub),
		client:  httpclient.New().WithTimeout(30 * time.Second).WithInsecureSkipVerify(),
	}
}

func (h *HiddifyClient) PanelType() string { return "hiddify" }

func (h *HiddifyClient) Authenticate(ctx context.Context) error {
	// Hiddify v2 uses API-key/basic auth per request; no session token required.
	if h.apiKey == "" {
		return fmt.Errorf("hiddify api key not configured")
	}
	return nil
}

func (h *HiddifyClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := h.Authenticate(ctx); err != nil {
		return nil, err
	}

	users, err := h.listUsers(ctx)
	if err != nil {
		return nil, err
	}

	var match map[string]interface{}
	for _, item := range users {
		name := strings.TrimSpace(fmt.Sprintf("%v", item["name"]))
		if strings.EqualFold(name, username) {
			match = item
			break
		}
	}
	if len(match) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	usageLimitGB := toFloat64(match["usage_limit_GB"])
	currentUsageGB := toFloat64(match["current_usage_GB"])
	dataLimit := int64(usageLimitGB * math.Pow(1024, 3))
	usedTraffic := int64(currentUsageGB * math.Pow(1024, 3))

	startDate := strings.TrimSpace(fmt.Sprintf("%v", match["start_date"]))
	if startDate == "<nil>" {
		startDate = ""
	}
	packageDays := int(toInt64(match["package_days"]))
	expireUnix := int64(0)
	if startDate != "" {
		if t := parseAnyTime(startDate); t > 0 && packageDays > 0 {
			expireUnix = t + int64(packageDays)*86400
		}
	}

	status := "active"
	if startDate == "" {
		status = "on_hold"
	} else if expireUnix > 0 && time.Now().Unix() >= expireUnix {
		status = "expired"
	} else if dataLimit > 0 && dataLimit-usedTraffic <= 0 {
		status = "limited"
	}

	onlineAt := strings.TrimSpace(fmt.Sprintf("%v", match["last_online"]))
	if onlineAt == "1-01-01 00:00:00" || onlineAt == "<nil>" {
		onlineAt = ""
	}

	subLink := ""
	uuidRaw := strings.TrimSpace(fmt.Sprintf("%v", match["uuid"]))
	if uuidRaw != "" && uuidRaw != "<nil>" {
		base := strings.TrimRight(h.linkSub, "/")
		if base == "" {
			base = h.baseURL
		}
		subLink = base + "/" + strings.Trim(uuidRaw, "/") + "/"
	}

	return &PanelUser{
		Username:    strings.TrimSpace(fmt.Sprintf("%v", match["name"])),
		Status:      status,
		DataLimit:   dataLimit,
		UsedTraffic: usedTraffic,
		ExpireTime:  expireUnix,
		SubLink:     subLink,
		OnlineAt:    parseAnyTime(onlineAt),
		Links:       []string{},
	}, nil
}

func (h *HiddifyClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := h.Authenticate(ctx); err != nil {
		return nil, err
	}

	packageDays := req.ExpireDays
	if packageDays <= 0 {
		packageDays = 111111
	}

	payload := map[string]interface{}{
		"uuid":             uuid.NewString(),
		"name":             req.Username,
		"added_by_uuid":    h.apiKey,
		"current_usage_GB": 0,
		"usage_limit_GB":   float64(req.DataLimit) / math.Pow(1024, 3),
		"package_days":     packageDays,
		"comment":          req.Note,
	}
	body, _ := json.Marshal(payload)

	httpc := h.client.Raw()
	resp, err := httpc.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("Hiddify-API-Key", h.apiKey).
		SetBody(body).
		Post(h.baseURL + "/api/v2/admin/user/")
	if err != nil {
		return nil, fmt.Errorf("hiddify create user failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, fmt.Errorf("hiddify create parse failed: %w", err)
	}
	if msg := strings.TrimSpace(getString(raw, "message")); msg != "" {
		return nil, fmt.Errorf(msg)
	}

	return h.GetUser(ctx, req.Username)
}

func (h *HiddifyClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	if err := h.Authenticate(ctx); err != nil {
		return nil, err
	}

	current, err := h.getRawUser(ctx, username)
	if err != nil {
		return nil, err
	}
	userUUID := strings.TrimSpace(fmt.Sprintf("%v", current["uuid"]))
	if userUUID == "" {
		return nil, fmt.Errorf("user uuid not found")
	}

	payload := map[string]interface{}{}
	if req.DataLimit > 0 {
		payload["usage_limit_GB"] = float64(req.DataLimit) / math.Pow(1024, 3)
	}
	if req.ExpireTime > 0 {
		delta := req.ExpireTime - time.Now().Unix()
		days := int(math.Ceil(float64(delta) / 86400))
		if days < 0 {
			days = 0
		}
		payload["package_days"] = days
	}
	if req.Note != "" {
		payload["comment"] = req.Note
	}
	if req.Status != "" {
		switch strings.ToLower(strings.TrimSpace(req.Status)) {
		case "active":
			payload["is_active"] = true
		case "disable", "disabled":
			payload["is_active"] = false
		}
	}
	if len(payload) == 0 {
		return h.GetUser(ctx, username)
	}

	body, _ := json.Marshal(payload)
	httpc := h.client.Raw()
	resp, err := httpc.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("Hiddify-API-Key", h.apiKey).
		SetBody(body).
		Patch(h.baseURL + "/api/v2/admin/user/" + userUUID + "/")
	if err != nil {
		return nil, fmt.Errorf("hiddify modify user failed: %w", err)
	}
	_ = resp

	return h.GetUser(ctx, username)
}

func (h *HiddifyClient) DeleteUser(ctx context.Context, username string) error {
	if err := h.Authenticate(ctx); err != nil {
		return err
	}
	current, err := h.getRawUser(ctx, username)
	if err != nil {
		return err
	}
	userUUID := strings.TrimSpace(fmt.Sprintf("%v", current["uuid"]))
	if userUUID == "" {
		return fmt.Errorf("user uuid not found")
	}

	httpc := h.client.Raw()
	_, err = httpc.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("Hiddify-API-Key", h.apiKey).
		Delete(h.baseURL + "/api/v2/admin/user/" + userUUID + "/")
	return err
}

func (h *HiddifyClient) EnableUser(ctx context.Context, username string) error {
	_, err := h.ModifyUser(ctx, username, ModifyUserRequest{Status: "active"})
	return err
}

func (h *HiddifyClient) DisableUser(ctx context.Context, username string) error {
	_, err := h.ModifyUser(ctx, username, ModifyUserRequest{Status: "disabled"})
	return err
}

func (h *HiddifyClient) ResetTraffic(ctx context.Context, username string) error {
	_, err := h.ModifyUser(ctx, username, ModifyUserRequest{})
	if err != nil {
		return err
	}
	current, err := h.getRawUser(ctx, username)
	if err != nil {
		return err
	}
	userUUID := strings.TrimSpace(fmt.Sprintf("%v", current["uuid"]))
	if userUUID == "" {
		return fmt.Errorf("user uuid not found")
	}
	payload := map[string]interface{}{"current_usage_GB": 0}
	body, _ := json.Marshal(payload)
	httpc := h.client.Raw()
	_, err = httpc.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("Hiddify-API-Key", h.apiKey).
		SetBody(body).
		Patch(h.baseURL + "/api/v2/admin/user/" + userUUID + "/")
	return err
}

func (h *HiddifyClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	user, err := h.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return user.SubLink, nil
}

func (h *HiddifyClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	return []PanelInbound{}, nil
}

func (h *HiddifyClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	if err := h.Authenticate(ctx); err != nil {
		return nil, err
	}
	httpc := h.client.Raw()
	resp, err := httpc.R().
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(h.apiKey+":"))).
		Get(h.baseURL + "/api/v2/admin/server_status/")
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (h *HiddifyClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	user, err := h.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return user.SubLink, nil
}

func (h *HiddifyClient) listUsers(ctx context.Context) ([]map[string]interface{}, error) {
	httpc := h.client.Raw()
	resp, err := httpc.R().
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(h.apiKey+":"))).
		Get(h.baseURL + "/api/v2/admin/user/")
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, err
	}

	switch t := raw.(type) {
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out, nil
	case map[string]interface{}:
		if msg := strings.TrimSpace(getString(t, "message")); msg != "" {
			return nil, fmt.Errorf(msg)
		}
		// Some versions return {"data":[...]}
		if arr, ok := t["data"].([]interface{}); ok {
			out := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					out = append(out, m)
				}
			}
			return out, nil
		}
		// Fallback: object itself as single user.
		return []map[string]interface{}{t}, nil
	default:
		return nil, fmt.Errorf("unexpected hiddify users response")
	}
}

func (h *HiddifyClient) getRawUser(ctx context.Context, username string) (map[string]interface{}, error) {
	users, err := h.listUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range users {
		name := strings.TrimSpace(fmt.Sprintf("%v", item["name"]))
		if strings.EqualFold(name, username) {
			return item, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}

func toFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := json.Number(strings.TrimSpace(t)).Float64()
		return f
	default:
		return 0
	}
}
