package panel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hosibot/internal/pkg/httpclient"
)

type xuiClient struct {
	baseURL         string
	username        string
	password        string
	apiBase         string
	defaultInbound  int
	linkSub         string
	startOnFirstUse bool
	client          *httpclient.Client
}

func NewXUIClient(baseURL, username, password, inboundID, linkSub string, startOnFirstUse bool) PanelClient {
	return newXUIClient(baseURL, username, password, "/panel/api/inbounds", inboundID, linkSub, startOnFirstUse, "x-ui_single")
}

func NewAlirezaSingleClient(baseURL, username, password, inboundID, linkSub string, startOnFirstUse bool) PanelClient {
	return newXUIClient(baseURL, username, password, "/xui/API/inbounds", inboundID, linkSub, startOnFirstUse, "alireza_single")
}

func newXUIClient(baseURL, username, password, apiBase, inboundID, linkSub string, startOnFirstUse bool, panelType string) PanelClient {
	id, _ := strconv.Atoi(strings.TrimSpace(inboundID))
	if id <= 0 {
		id = 1
	}
	return &xuiClient{
		baseURL:         strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username:        strings.TrimSpace(username),
		password:        password,
		apiBase:         apiBase,
		defaultInbound:  id,
		linkSub:         strings.TrimSpace(linkSub),
		startOnFirstUse: startOnFirstUse,
		client:          httpclient.New().WithTimeout(30*time.Second).WithInsecureSkipVerify().WithHeader("Accept", "application/json"),
	}
}

func (x *xuiClient) PanelType() string {
	if strings.Contains(x.apiBase, "/xui/") {
		return "alireza_single"
	}
	return "x-ui_single"
}

func (x *xuiClient) Authenticate(ctx context.Context) error {
	httpc := x.client.Raw()
	_, err := httpc.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetFormData(map[string]string{
			"username": x.username,
			"password": x.password,
		}).
		Post(x.baseURL + "/login")
	if err != nil {
		return fmt.Errorf("xui auth failed: %w", err)
	}
	return nil
}

func (x *xuiClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := x.Authenticate(ctx); err != nil {
		return nil, err
	}
	row, err := x.fetchClientTraffic(username)
	if err != nil {
		return nil, err
	}
	return x.toPanelUser(row), nil
}

func (x *xuiClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := x.Authenticate(ctx); err != nil {
		return nil, err
	}

	inboundID := x.defaultInbound
	if id := pickInboundID(req.Inbounds); id > 0 {
		inboundID = id
	}

	expiry := int64(0)
	if req.ExpireDays > 0 {
		expiry = time.Now().Add(time.Duration(req.ExpireDays)*24*time.Hour).Unix() * 1000
		if x.startOnFirstUse {
			// XUI convention used by legacy PHP.
			expiry = -int64(req.ExpireDays) * 86400000
		}
	}

	subID := randomHex(8)
	settings := map[string]interface{}{
		"clients": []map[string]interface{}{
			{
				"id":         uuid.NewString(),
				"flow":       "",
				"email":      req.Username,
				"totalGB":    req.DataLimit,
				"expiryTime": expiry,
				"enable":     true,
				"tgId":       "",
				"subId":      subID,
				"reset":      0,
				"comment":    req.Note,
			},
		},
		"decryption": "none",
		"fallbacks":  []interface{}{},
	}
	settingsJSON, _ := json.Marshal(settings)
	payload := map[string]interface{}{
		"id":       inboundID,
		"settings": string(settingsJSON),
	}

	httpc := x.client.Raw()
	resp, err := httpc.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(x.baseURL + x.apiBase + "/addClient")
	if err != nil {
		return nil, fmt.Errorf("xui create user failed: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, fmt.Errorf("xui create parse failed: %w", err)
	}
	if ok, _ := raw["success"].(bool); !ok {
		return nil, fmt.Errorf("xui create rejected")
	}

	return x.GetUser(ctx, req.Username)
}

func (x *xuiClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	if err := x.Authenticate(ctx); err != nil {
		return nil, err
	}
	current, err := x.fetchClientTraffic(username)
	if err != nil {
		return nil, err
	}

	inboundID := int(toInt64(current["inboundId"]))
	if inboundID <= 0 {
		inboundID = x.defaultInbound
	}
	clientID := strings.TrimSpace(fmt.Sprintf("%v", current["id"]))
	if clientID == "" || clientID == "<nil>" {
		clientID = strings.TrimSpace(fmt.Sprintf("%v", current["clientId"]))
	}

	enable := boolFromAny(current["enable"], true)
	if req.Status != "" {
		switch strings.ToLower(strings.TrimSpace(req.Status)) {
		case "active":
			enable = true
		case "disable", "disabled":
			enable = false
		}
	}

	dataLimit := toInt64(current["total"])
	if req.DataLimit > 0 {
		dataLimit = req.DataLimit
	}

	expiry := toInt64(current["expiryTime"])
	if req.ExpireTime > 0 {
		expiry = req.ExpireTime * 1000
	}

	settings := map[string]interface{}{
		"clients": []map[string]interface{}{
			{
				"id":         current["id"],
				"flow":       current["flow"],
				"email":      current["email"],
				"totalGB":    dataLimit,
				"expiryTime": expiry,
				"enable":     enable,
				"subId":      current["subId"],
				"comment":    req.Note,
			},
		},
		"decryption": "none",
		"fallbacks":  []interface{}{},
	}
	settingsJSON, _ := json.Marshal(settings)
	payload := map[string]interface{}{
		"id":       inboundID,
		"settings": string(settingsJSON),
	}

	updatePath := x.baseURL + x.apiBase + "/updateClient/" + clientID
	httpc := x.client.Raw()
	resp, err := httpc.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(updatePath)
	if err != nil {
		return nil, fmt.Errorf("xui modify user failed: %w", err)
	}
	var raw map[string]interface{}
	_ = json.Unmarshal(resp.Body(), &raw)
	if ok, _ := raw["success"].(bool); !ok {
		return nil, fmt.Errorf("xui modify rejected")
	}

	return x.GetUser(ctx, username)
}

func (x *xuiClient) DeleteUser(ctx context.Context, username string) error {
	if err := x.Authenticate(ctx); err != nil {
		return err
	}
	current, err := x.fetchClientTraffic(username)
	if err != nil {
		return err
	}
	inboundID := int(toInt64(current["inboundId"]))
	if inboundID <= 0 {
		inboundID = x.defaultInbound
	}
	path := fmt.Sprintf("%s%s/%d/delClientByEmail/%s", x.baseURL, x.apiBase, inboundID, username)
	_, err = x.client.Raw().R().Post(path)
	return err
}

func (x *xuiClient) EnableUser(ctx context.Context, username string) error {
	_, err := x.ModifyUser(ctx, username, ModifyUserRequest{Status: "active"})
	return err
}

func (x *xuiClient) DisableUser(ctx context.Context, username string) error {
	_, err := x.ModifyUser(ctx, username, ModifyUserRequest{Status: "disabled"})
	return err
}

func (x *xuiClient) ResetTraffic(ctx context.Context, username string) error {
	if err := x.Authenticate(ctx); err != nil {
		return err
	}
	current, err := x.fetchClientTraffic(username)
	if err != nil {
		return err
	}
	inboundID := int(toInt64(current["inboundId"]))
	if inboundID <= 0 {
		inboundID = x.defaultInbound
	}
	path := fmt.Sprintf("%s%s/%d/resetClientTraffic/%s", x.baseURL, x.apiBase, inboundID, username)
	_, err = x.client.Raw().R().Post(path)
	return err
}

func (x *xuiClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	user, err := x.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return user.SubLink, nil
}

func (x *xuiClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	if err := x.Authenticate(ctx); err != nil {
		return nil, err
	}
	resp, err := x.client.Raw().R().Get(x.baseURL + x.apiBase)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, err
	}
	items, _ := raw["obj"].([]interface{})
	out := make([]PanelInbound, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, PanelInbound{
			Tag:      strings.TrimSpace(fmt.Sprintf("%v", m["id"])),
			Protocol: strings.TrimSpace(fmt.Sprintf("%v", m["protocol"])),
			Port:     int(toInt64(m["port"])),
			Remark:   strings.TrimSpace(fmt.Sprintf("%v", m["remark"])),
		})
	}
	return out, nil
}

func (x *xuiClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (x *xuiClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	user, err := x.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	return user.SubLink, nil
}

func (x *xuiClient) fetchClientTraffic(username string) (map[string]interface{}, error) {
	primaryPath := x.baseURL + x.apiBase + "/getClientTraffics/" + username
	resp, err := x.client.Raw().R().Get(primaryPath)
	if err == nil {
		var raw map[string]interface{}
		if uErr := json.Unmarshal(resp.Body(), &raw); uErr == nil {
			if ok, _ := raw["success"].(bool); ok {
				if obj, ok := raw["obj"].(map[string]interface{}); ok && len(obj) > 0 {
					return obj, nil
				}
			}
		}
	}

	// Fallback for panels that only expose full inbound list.
	resp, err = x.client.Raw().R().Get(x.baseURL + x.apiBase)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, err
	}
	items, _ := raw["obj"].([]interface{})
	for _, item := range items {
		inbound, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		settingsStr := strings.TrimSpace(fmt.Sprintf("%v", inbound["settings"]))
		var settings map[string]interface{}
		_ = json.Unmarshal([]byte(settingsStr), &settings)
		clients, _ := settings["clients"].([]interface{})
		stats, _ := inbound["clientStats"].([]interface{})

		var clientItem map[string]interface{}
		for _, c := range clients {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", cm["email"])), username) {
				clientItem = cm
				break
			}
		}
		if len(clientItem) == 0 {
			continue
		}

		row := map[string]interface{}{
			"inboundId": inbound["id"],
			"id":        clientItem["id"],
			"email":     clientItem["email"],
			"flow":      clientItem["flow"],
			"subId":     clientItem["subId"],
			"expiryTime": func() interface{} {
				if v, ok := clientItem["expiryTime"]; ok {
					return v
				}
				return int64(0)
			}(),
			"enable": func() interface{} {
				if v, ok := clientItem["enable"]; ok {
					return v
				}
				return true
			}(),
			"total": clientItem["totalGB"],
		}

		for _, st := range stats {
			sm, ok := st.(map[string]interface{})
			if !ok {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", sm["email"])), username) {
				row["up"] = sm["up"]
				row["down"] = sm["down"]
				row["lastOnline"] = sm["lastOnline"]
				break
			}
		}
		return row, nil
	}
	return nil, fmt.Errorf("user not found")
}

func (x *xuiClient) toPanelUser(row map[string]interface{}) *PanelUser {
	expiryMS := toInt64(row["expiryTime"])
	expire := int64(0)
	if expiryMS > 0 {
		expire = expiryMS / 1000
	}
	status := "active"
	if !boolFromAny(row["enable"], true) {
		status = "disabled"
	}
	total := toInt64(row["total"])
	used := toInt64(row["up"]) + toInt64(row["down"])
	if total > 0 && total-used <= 0 {
		status = "limited"
	}
	if expire > 0 && expire <= time.Now().Unix() {
		status = "expired"
	}
	if expiryMS < -10000 {
		status = "on_hold"
	}

	subID := strings.TrimSpace(fmt.Sprintf("%v", row["subId"]))
	subLink := ""
	if subID != "" && x.linkSub != "" {
		subLink = strings.TrimRight(x.linkSub, "/") + "/" + subID
	}

	return &PanelUser{
		Username:    strings.TrimSpace(fmt.Sprintf("%v", row["email"])),
		Status:      status,
		DataLimit:   total,
		UsedTraffic: used,
		ExpireTime:  expire,
		SubLink:     subLink,
		OnlineAt:    toInt64(row["lastOnline"]) / 1000,
		Links:       []string{},
	}
}

func pickInboundID(inbounds map[string][]string) int {
	for proto, tags := range inbounds {
		if len(tags) == 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(proto)); err == nil && n > 0 {
				return n
			}
		}
		for _, tag := range tags {
			if n, err := strconv.Atoi(strings.TrimSpace(tag)); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

func randomHex(size int) string {
	if size <= 0 {
		size = 8
	}
	b := make([]byte, size)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func boolFromAny(v interface{}, defaultVal bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return defaultVal
}
