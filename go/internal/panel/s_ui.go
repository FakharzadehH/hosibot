package panel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hosibot/internal/pkg/httpclient"
)

type SUIClient struct {
	baseURL string
	token   string
	client  *httpclient.Client
}

func NewSUIClient(baseURL, username, password string) PanelClient {
	token := strings.TrimSpace(password)
	if token == "" {
		token = strings.TrimSpace(username)
	}
	return &SUIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		client:  httpclient.New().WithTimeout(25 * time.Second).WithInsecureSkipVerify(),
	}
}

func (s *SUIClient) PanelType() string { return "s_ui" }

func (s *SUIClient) Authenticate(ctx context.Context) error {
	if s.token == "" {
		return fmt.Errorf("s-ui token missing")
	}
	_, err := s.req().Get(s.baseURL + "/apiv2/settings")
	return err
}

func (s *SUIClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := s.Authenticate(ctx); err != nil {
		return nil, err
	}

	clientLite, err := s.findClient(username)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", clientLite["id"]))
	if id == "" {
		return nil, fmt.Errorf("user id not found")
	}

	detail, err := s.getClientByID(id)
	if err != nil {
		return nil, err
	}

	enable := boolFromAny(detail["enable"], true)
	volume := toInt64(detail["volume"])
	up := toInt64(detail["up"])
	down := toInt64(detail["down"])
	expiry := toInt64(detail["expiry"])

	status := "active"
	if !enable {
		status = "disabled"
	}
	if volume > 0 && volume-(up+down) <= 0 {
		status = "limited"
	}
	if expiry > 0 && time.Now().Unix() >= expiry {
		status = "expired"
	}

	subLink := s.subscriptionURL(strings.TrimSpace(fmt.Sprintf("%v", detail["name"])))

	return &PanelUser{
		Username:    strings.TrimSpace(fmt.Sprintf("%v", detail["name"])),
		Status:      status,
		DataLimit:   volume,
		UsedTraffic: up + down,
		ExpireTime:  expiry,
		SubLink:     subLink,
		Links:       []string{},
	}, nil
}

func (s *SUIClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := s.Authenticate(ctx); err != nil {
		return nil, err
	}

	expiry := int64(0)
	if req.ExpireDays > 0 {
		expiry = time.Now().Add(time.Duration(req.ExpireDays) * 24 * time.Hour).Unix()
	}

	payloadData := map[string]interface{}{
		"enable": true,
		"name":   req.Username,
		"config": s.newClientConfig(req.Username),
		"inbounds": func() interface{} {
			if len(req.Inbounds) > 0 {
				out := map[string][]string{}
				for k, v := range req.Inbounds {
					out[k] = v
				}
				return out
			}
			return map[string][]string{}
		}(),
		"links":  []string{},
		"volume": req.DataLimit,
		"expiry": expiry,
		"desc":   req.Note,
	}
	rawData, _ := json.Marshal(payloadData)
	form := map[string]string{
		"object": "clients",
		"action": "new",
		"data":   string(rawData),
	}
	resp, err := s.req().Raw().R().
		SetFormData(form).
		Post(s.baseURL + "/apiv2/save")
	if err != nil {
		return nil, fmt.Errorf("s-ui create failed: %w", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		return nil, fmt.Errorf("s-ui create parse failed: %w", err)
	}
	if ok, _ := out["success"].(bool); !ok {
		return nil, fmt.Errorf("s-ui create rejected")
	}

	return s.GetUser(ctx, req.Username)
}

func (s *SUIClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	if err := s.Authenticate(ctx); err != nil {
		return nil, err
	}
	current, err := s.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}
	clientLite, err := s.findClient(username)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", clientLite["id"]))
	detail, err := s.getClientByID(id)
	if err != nil {
		return nil, err
	}

	enable := true
	switch strings.ToLower(strings.TrimSpace(req.Status)) {
	case "disable", "disabled":
		enable = false
	case "active":
		enable = true
	default:
		enable = boolFromAny(detail["enable"], true)
	}

	volume := current.DataLimit
	if req.DataLimit > 0 {
		volume = req.DataLimit
	}
	expiry := current.ExpireTime
	if req.ExpireTime > 0 {
		expiry = req.ExpireTime
	}
	desc := strings.TrimSpace(fmt.Sprintf("%v", detail["desc"]))
	if req.Note != "" {
		desc = req.Note
	}

	payloadData := map[string]interface{}{
		"id":       id,
		"enable":   enable,
		"name":     username,
		"config":   detail["config"],
		"inbounds": detail["inbounds"],
		"links":    detail["links"],
		"volume":   volume,
		"expiry":   expiry,
		"desc":     desc,
		"up":       detail["up"],
		"down":     detail["down"],
	}
	rawData, _ := json.Marshal(payloadData)
	form := map[string]string{
		"object": "clients",
		"action": "edit",
		"data":   string(rawData),
	}

	resp, err := s.req().Raw().R().
		SetFormData(form).
		Post(s.baseURL + "/apiv2/save")
	if err != nil {
		return nil, fmt.Errorf("s-ui modify failed: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		return nil, fmt.Errorf("s-ui modify parse failed: %w", err)
	}
	if ok, _ := out["success"].(bool); !ok {
		return nil, fmt.Errorf("s-ui modify rejected")
	}

	return s.GetUser(ctx, username)
}

func (s *SUIClient) DeleteUser(ctx context.Context, username string) error {
	if err := s.Authenticate(ctx); err != nil {
		return err
	}
	clientLite, err := s.findClient(username)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", clientLite["id"]))
	form := map[string]string{
		"object": "clients",
		"action": "del",
		"data":   id,
	}
	_, err = s.req().Raw().R().
		SetFormData(form).
		Post(s.baseURL + "/apiv2/save")
	return err
}

func (s *SUIClient) EnableUser(ctx context.Context, username string) error {
	_, err := s.ModifyUser(ctx, username, ModifyUserRequest{Status: "active"})
	return err
}

func (s *SUIClient) DisableUser(ctx context.Context, username string) error {
	_, err := s.ModifyUser(ctx, username, ModifyUserRequest{Status: "disabled"})
	return err
}

func (s *SUIClient) ResetTraffic(ctx context.Context, username string) error {
	if err := s.Authenticate(ctx); err != nil {
		return err
	}
	clientLite, err := s.findClient(username)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", clientLite["id"]))
	detail, err := s.getClientByID(id)
	if err != nil {
		return err
	}
	payloadData := map[string]interface{}{
		"id":       id,
		"enable":   detail["enable"],
		"name":     detail["name"],
		"config":   detail["config"],
		"inbounds": detail["inbounds"],
		"links":    detail["links"],
		"volume":   detail["volume"],
		"expiry":   detail["expiry"],
		"desc":     detail["desc"],
		"up":       0,
		"down":     0,
	}
	rawData, _ := json.Marshal(payloadData)
	form := map[string]string{
		"object": "clients",
		"action": "edit",
		"data":   string(rawData),
	}
	_, err = s.req().Raw().R().
		SetFormData(form).
		Post(s.baseURL + "/apiv2/save")
	return err
}

func (s *SUIClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	return s.subscriptionURL(username), nil
}

func (s *SUIClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	// S-UI represents inbounds as complex config object; expose empty list.
	return []PanelInbound{}, nil
}

func (s *SUIClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	resp, err := s.req().Get(s.baseURL + "/apiv2/settings")
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *SUIClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	return s.subscriptionURL(username), nil
}

func (s *SUIClient) req() *httpclient.Client {
	return s.client.WithHeader("Token", s.token)
}

func (s *SUIClient) findClient(username string) (map[string]interface{}, error) {
	resp, err := s.req().Get(s.baseURL + "/apiv2/clients")
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	if ok, _ := raw["success"].(bool); !ok {
		return nil, fmt.Errorf("s-ui clients rejected")
	}
	obj, _ := raw["obj"].(map[string]interface{})
	clients, _ := obj["clients"].([]interface{})
	for _, item := range clients {
		client, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := strings.TrimSpace(fmt.Sprintf("%v", client["name"]))
		if strings.EqualFold(name, username) {
			return client, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}

func (s *SUIClient) getClientByID(id string) (map[string]interface{}, error) {
	resp, err := s.req().Get(s.baseURL + "/apiv2/clients?id=" + url.QueryEscape(id))
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	if ok, _ := raw["success"].(bool); !ok {
		return nil, fmt.Errorf("s-ui client detail rejected")
	}
	obj, _ := raw["obj"].(map[string]interface{})
	clients, _ := obj["clients"].([]interface{})
	if len(clients) == 0 {
		return nil, fmt.Errorf("client not found")
	}
	first, _ := clients[0].(map[string]interface{})
	return first, nil
}

func (s *SUIClient) subscriptionURL(username string) string {
	resp, err := s.req().Get(s.baseURL + "/apiv2/settings")
	if err != nil {
		return ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return ""
	}
	obj, _ := raw["obj"].(map[string]interface{})
	subPort := strings.TrimSpace(fmt.Sprintf("%v", obj["subPort"]))
	subPath := strings.TrimSpace(fmt.Sprintf("%v", obj["subPath"]))
	if subPort == "" || subPort == "<nil>" {
		return ""
	}
	base, err := url.Parse(s.baseURL)
	if err != nil {
		return ""
	}
	scheme := base.Scheme
	host := base.Hostname()
	if scheme == "" {
		scheme = "http"
	}
	if host == "" {
		host = strings.Trim(strings.Split(s.baseURL, ":")[0], "/")
	}
	if !strings.HasPrefix(subPath, "/") {
		subPath = "/" + subPath
	}
	if strings.HasSuffix(subPath, "/") {
		subPath = strings.TrimRight(subPath, "/")
	}
	if _, err := strconv.Atoi(subPort); err != nil {
		return ""
	}
	return fmt.Sprintf("%s://%s:%s%s/%s", scheme, host, subPort, subPath, username)
}

func (s *SUIClient) newClientConfig(username string) map[string]interface{} {
	pass := randomToken(16)
	return map[string]interface{}{
		"mixed": map[string]interface{}{
			"username": username,
			"password": randomToken(10),
		},
		"socks": map[string]interface{}{
			"username": username,
			"password": randomToken(10),
		},
		"http": map[string]interface{}{
			"username": username,
			"password": randomToken(10),
		},
		"shadowsocks": map[string]interface{}{
			"name":     username,
			"password": pass,
		},
		"shadowsocks16": map[string]interface{}{
			"name":     username,
			"password": pass,
		},
		"shadowtls": map[string]interface{}{
			"name":     username,
			"password": pass,
		},
		"vmess": map[string]interface{}{
			"name":    username,
			"uuid":    uuid.NewString(),
			"alterId": 0,
		},
		"vless": map[string]interface{}{
			"name": username,
			"uuid": uuid.NewString(),
			"flow": "",
		},
		"trojan": map[string]interface{}{
			"name":     username,
			"password": randomToken(10),
		},
		"naive": map[string]interface{}{
			"username": username,
			"password": randomToken(10),
		},
		"hysteria": map[string]interface{}{
			"name":     username,
			"auth_str": randomToken(10),
		},
		"tuic": map[string]interface{}{
			"name":     username,
			"uuid":     uuid.NewString(),
			"password": randomToken(10),
		},
		"hysteria2": map[string]interface{}{
			"name":     username,
			"password": randomToken(10),
		},
	}
}

func randomToken(size int) string {
	if size <= 0 {
		size = 10
	}
	b := make([]byte, size)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
