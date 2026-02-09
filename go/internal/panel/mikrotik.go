package panel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"hosibot/internal/pkg/httpclient"
)

type MikrotikClient struct {
	baseURL        string
	username       string
	password       string
	defaultProfile string
	client         *httpclient.Client
}

func NewMikrotikClient(baseURL, username, password, defaultProfile string) PanelClient {
	return &MikrotikClient{
		baseURL:        strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username:       strings.TrimSpace(username),
		password:       password,
		defaultProfile: strings.TrimSpace(defaultProfile),
		client:         httpclient.New().WithTimeout(20 * time.Second).WithInsecureSkipVerify(),
	}
}

func (m *MikrotikClient) PanelType() string { return "mikrotik" }

func (m *MikrotikClient) req() *httpclient.Client {
	return m.client
}

func (m *MikrotikClient) Authenticate(ctx context.Context) error {
	resp, err := m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		Get(m.baseURL + "/rest/system/resource")
	if err != nil {
		return fmt.Errorf("mikrotik auth failed: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return fmt.Errorf("mikrotik auth parse failed: %w", err)
	}
	return nil
}

func (m *MikrotikClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := m.Authenticate(ctx); err != nil {
		return nil, err
	}
	userID, err := m.getUserID(ctx, username)
	if err != nil {
		return nil, err
	}

	usedTraffic := int64(0)
	monitorPayload := map[string]interface{}{
		"once": true,
		".id":  userID,
	}
	resp, err := m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		SetHeader("Content-Type", "application/json").
		SetBody(monitorPayload).
		Post(m.baseURL + "/rest/user-manager/user/monitor")
	if err == nil {
		var rows []map[string]interface{}
		if json.Unmarshal(resp.Body(), &rows) == nil && len(rows) > 0 {
			usedTraffic = toInt64(rows[0]["bytes-in"]) + toInt64(rows[0]["bytes-out"])
		}
	}

	return &PanelUser{
		Username:    username,
		Status:      "active",
		DataLimit:   0,
		UsedTraffic: usedTraffic,
		ExpireTime:  0,
		Links:       []string{},
	}, nil
}

func (m *MikrotikClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := m.Authenticate(ctx); err != nil {
		return nil, err
	}

	password := randomHexString(6)
	payload := map[string]interface{}{
		"name":     req.Username,
		"password": password,
	}
	_, err := m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(m.baseURL + "/rest/user-manager/user/add")
	if err != nil {
		return nil, fmt.Errorf("mikrotik create user failed: %w", err)
	}

	profile := strings.TrimSpace(m.defaultProfile)
	if p := pickProfile(req.Inbounds); p != "" {
		profile = p
	}
	if profile != "" {
		_, _ = m.req().Raw().R().
			SetBasicAuth(m.username, m.password).
			SetHeader("Content-Type", "application/json").
			SetBody(map[string]interface{}{
				"user":    req.Username,
				"profile": profile,
			}).
			Post(m.baseURL + "/rest/user-manager/user-profile/add")
	}

	return &PanelUser{
		Username: req.Username,
		Status:   "active",
		SubLink:  password,
		Links:    []string{},
	}, nil
}

func (m *MikrotikClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	// RouterOS REST does not expose stable cross-version user-manager update fields.
	// Keep behavior non-destructive and return current state.
	return m.GetUser(ctx, username)
}

func (m *MikrotikClient) DeleteUser(ctx context.Context, username string) error {
	if err := m.Authenticate(ctx); err != nil {
		return err
	}
	userID, err := m.getUserID(ctx, username)
	if err != nil {
		return err
	}
	payload := map[string]interface{}{".id": userID}
	_, err = m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(m.baseURL + "/rest/user-manager/user/remove")
	return err
}

func (m *MikrotikClient) EnableUser(ctx context.Context, username string) error {
	return nil
}

func (m *MikrotikClient) DisableUser(ctx context.Context, username string) error {
	return nil
}

func (m *MikrotikClient) ResetTraffic(ctx context.Context, username string) error {
	return nil
}

func (m *MikrotikClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	return "", nil
}

func (m *MikrotikClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	if err := m.Authenticate(ctx); err != nil {
		return nil, err
	}
	resp, err := m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		Get(m.baseURL + "/rest/user-manager/profile")
	if err != nil {
		return nil, err
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &rows); err != nil {
		return nil, err
	}
	out := make([]PanelInbound, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(fmt.Sprintf("%v", row["name"]))
		if name == "" {
			continue
		}
		out = append(out, PanelInbound{
			Tag:      name,
			Protocol: "profile",
			Remark:   "mikrotik profile",
		})
	}
	return out, nil
}

func (m *MikrotikClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	if err := m.Authenticate(ctx); err != nil {
		return nil, err
	}
	resp, err := m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		Get(m.baseURL + "/rest/system/resource")
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (m *MikrotikClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	return "", nil
}

func (m *MikrotikClient) getUserID(ctx context.Context, username string) (string, error) {
	resp, err := m.req().Raw().R().
		SetBasicAuth(m.username, m.password).
		Get(m.baseURL + "/rest/user-manager/user?name=" + url.QueryEscape(username))
	if err != nil {
		return "", err
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("user not found")
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", rows[0][".id"]))
	if id == "" {
		return "", fmt.Errorf("user id not found")
	}
	return id, nil
}

func pickProfile(inbounds map[string][]string) string {
	for key, tags := range inbounds {
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				return tag
			}
		}
		key = strings.TrimSpace(key)
		if key != "" {
			return key
		}
	}
	return ""
}

func randomHexString(size int) string {
	if size <= 0 {
		size = 6
	}
	buf := make([]byte, size)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
