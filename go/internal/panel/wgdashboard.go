package panel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/curve25519"

	"hosibot/internal/pkg/httpclient"
)

type WGDashboardClient struct {
	baseURL   string
	apiKey    string
	inboundID string
	client    *httpclient.Client
}

func NewWGDashboardClient(baseURL, apiKey, inboundID string) PanelClient {
	return &WGDashboardClient{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:    strings.TrimSpace(apiKey),
		inboundID: strings.TrimSpace(inboundID),
		client:    httpclient.New().WithTimeout(25 * time.Second).WithInsecureSkipVerify(),
	}
}

func (w *WGDashboardClient) PanelType() string { return "wgdashboard" }

func (w *WGDashboardClient) Authenticate(ctx context.Context) error {
	if w.apiKey == "" {
		return fmt.Errorf("wgdashboard api key not configured")
	}
	if w.inboundID == "" {
		return fmt.Errorf("wgdashboard inbound id not configured")
	}
	_, err := w.getConfigurationInfo()
	return err
}

func (w *WGDashboardClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := w.Authenticate(ctx); err != nil {
		return nil, err
	}

	peer, restricted, err := w.findPeer(username)
	if err != nil {
		return nil, err
	}
	publicKey := strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"]))
	if publicKey == "" {
		return nil, fmt.Errorf("public key not found for peer")
	}

	jobs := toMapSlice(peer["jobs"])
	dataLimitGB := jobValueFloat(jobs, "total_data")
	expireTime := parseWGDateJob(jobValueString(jobs, "date"))
	usedBytes := int64((toFloat64(peer["total_data"]) + toFloat64(peer["cumu_data"])) * float64(1024*1024*1024))

	dataLimitBytes := int64(0)
	if dataLimitGB > 0 {
		dataLimitBytes = int64(dataLimitGB * float64(1024*1024*1024))
	}

	status := "active"
	if restricted || !boolFromAny(mapValueMap(peer, "configuration")["Status"], true) {
		status = "disabled"
	}
	if expireTime > 0 && expireTime <= time.Now().Unix() {
		status = "expired"
	}
	if dataLimitBytes > 0 && usedBytes >= dataLimitBytes {
		status = "limited"
	}

	downloadLink, _ := w.downloadPeer(publicKey)

	return &PanelUser{
		Username:    strings.TrimSpace(fmt.Sprintf("%v", peer["name"])),
		Status:      status,
		DataLimit:   dataLimitBytes,
		UsedTraffic: usedBytes,
		ExpireTime:  expireTime,
		SubLink:     downloadLink,
		OnlineAt:    parseAnyTime(peer["last_handshake"]),
		Note:        publicKey,
		Links:       []string{},
	}, nil
}

func (w *WGDashboardClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := w.Authenticate(ctx); err != nil {
		return nil, err
	}

	ips, err := w.availableIPs()
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no available ip found")
	}
	keys, err := newWGKeys()
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"name":          req.Username,
		"allowed_ips":   []string{ips[0]},
		"private_key":   keys.Private,
		"public_key":    keys.Public,
		"preshared_key": keys.Preshared,
	}
	body, _ := json.Marshal(payload)
	resp, err := w.req().Raw().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(w.baseURL + "/api/addPeers/" + url.PathEscape(w.inboundID))
	if err != nil {
		return nil, fmt.Errorf("wgdashboard add peer failed: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		return nil, fmt.Errorf("wgdashboard add peer parse failed: %w", err)
	}
	if ok, _ := out["status"].(bool); !ok {
		return nil, fmt.Errorf("wgdashboard add peer rejected")
	}

	if req.DataLimit > 0 {
		gb := float64(req.DataLimit) / float64(1024*1024*1024)
		_ = w.saveRestrictJob(keys.Public, "total_data", strconv.FormatFloat(gb, 'f', 2, 64))
	}
	if req.ExpireDays > 0 {
		exp := time.Now().Add(time.Duration(req.ExpireDays) * 24 * time.Hour).Format("2006-01-02 15:04:05")
		_ = w.saveRestrictJob(keys.Public, "date", exp)
	}

	return w.GetUser(ctx, req.Username)
}

func (w *WGDashboardClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	if err := w.Authenticate(ctx); err != nil {
		return nil, err
	}
	peer, _, err := w.findPeer(username)
	if err != nil {
		return nil, err
	}
	publicKey := strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"]))
	if publicKey == "" {
		return nil, fmt.Errorf("public key not found")
	}

	if req.Status != "" {
		switch strings.ToLower(strings.TrimSpace(req.Status)) {
		case "active":
			_ = w.allowAccess(publicKey)
		case "disable", "disabled":
			_ = w.restrictPeer(publicKey)
		}
	}
	if req.DataLimit > 0 {
		gb := float64(req.DataLimit) / float64(1024*1024*1024)
		_ = w.saveRestrictJob(publicKey, "total_data", strconv.FormatFloat(gb, 'f', 2, 64))
	}
	if req.ExpireTime > 0 {
		_ = w.saveRestrictJob(publicKey, "date", time.Unix(req.ExpireTime, 0).Format("2006-01-02 15:04:05"))
	}

	return w.GetUser(ctx, username)
}

func (w *WGDashboardClient) DeleteUser(ctx context.Context, username string) error {
	if err := w.Authenticate(ctx); err != nil {
		return err
	}
	peer, _, err := w.findPeer(username)
	if err != nil {
		return err
	}
	publicKey := strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"]))
	if publicKey == "" {
		return fmt.Errorf("public key not found")
	}
	_ = w.allowAccess(publicKey)
	body, _ := json.Marshal(map[string]interface{}{
		"peers": []string{publicKey},
	})
	_, err = w.req().Raw().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(w.baseURL + "/api/deletePeers/" + url.PathEscape(w.inboundID))
	return err
}

func (w *WGDashboardClient) EnableUser(ctx context.Context, username string) error {
	peer, _, err := w.findPeer(username)
	if err != nil {
		return err
	}
	return w.allowAccess(strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"])))
}

func (w *WGDashboardClient) DisableUser(ctx context.Context, username string) error {
	peer, _, err := w.findPeer(username)
	if err != nil {
		return err
	}
	return w.restrictPeer(strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"])))
}

func (w *WGDashboardClient) ResetTraffic(ctx context.Context, username string) error {
	peer, _, err := w.findPeer(username)
	if err != nil {
		return err
	}
	publicKey := strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"]))
	body, _ := json.Marshal(map[string]interface{}{
		"id":   publicKey,
		"type": "total",
	})
	_, err = w.req().Raw().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(w.baseURL + "/api/resetPeerData/" + url.PathEscape(w.inboundID))
	return err
}

func (w *WGDashboardClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	peer, _, err := w.findPeer(username)
	if err != nil {
		return "", err
	}
	publicKey := strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"]))
	return w.downloadPeer(publicKey)
}

func (w *WGDashboardClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	return []PanelInbound{{
		Tag:      w.inboundID,
		Protocol: "wireguard",
		Remark:   "wgdashboard configuration",
	}}, nil
}

func (w *WGDashboardClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	info, err := w.getConfigurationInfo()
	if err != nil {
		return nil, err
	}
	peers := toMapSlice(mapValueMap(info, "data")["configurationPeers"])
	restricted := toMapSlice(mapValueMap(info, "data")["configurationRestrictedPeers"])
	return map[string]interface{}{
		"peers":            len(peers),
		"restricted_peers": len(restricted),
		"configuration":    mapValueMap(info, "data")["configuration"],
	}, nil
}

func (w *WGDashboardClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	peer, _, err := w.findPeer(username)
	if err != nil {
		return "", err
	}
	return w.downloadPeer(strings.TrimSpace(fmt.Sprintf("%v", peer["public_key"])))
}

func (w *WGDashboardClient) req() *httpclient.Client {
	return w.client.WithHeader("Accept", "application/json").WithHeader("wg-dashboard-apikey", w.apiKey)
}

func (w *WGDashboardClient) getConfigurationInfo() (map[string]interface{}, error) {
	resp, err := w.req().Get(w.baseURL + "/api/getWireguardConfigurationInfo?configurationName=" + url.QueryEscape(w.inboundID))
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	if ok, _ := raw["status"].(bool); !ok {
		return nil, fmt.Errorf("wgdashboard request rejected")
	}
	return raw, nil
}

func (w *WGDashboardClient) findPeer(username string) (map[string]interface{}, bool, error) {
	info, err := w.getConfigurationInfo()
	if err != nil {
		return nil, false, err
	}
	data := mapValueMap(info, "data")
	for _, peer := range toMapSlice(data["configurationPeers"]) {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", peer["name"])), username) {
			peer["configuration"] = data["configuration"]
			return peer, false, nil
		}
	}
	for _, peer := range toMapSlice(data["configurationRestrictedPeers"]) {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", peer["name"])), username) {
			peer["configuration"] = map[string]interface{}{"Status": false}
			return peer, true, nil
		}
	}
	return nil, false, fmt.Errorf("user not found")
}

func (w *WGDashboardClient) availableIPs() ([]string, error) {
	resp, err := w.req().Get(w.baseURL + "/api/getAvailableIPs/" + url.PathEscape(w.inboundID))
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	if ok, _ := raw["status"].(bool); !ok {
		return nil, fmt.Errorf("wgdashboard get available ips rejected")
	}
	data := mapValueMap(raw, "data")
	out := make([]string, 0)
	for _, value := range data {
		switch t := value.(type) {
		case []interface{}:
			for _, ip := range t {
				s := strings.TrimSpace(fmt.Sprintf("%v", ip))
				if s != "" {
					out = append(out, s)
				}
			}
		case []string:
			for _, ip := range t {
				ip = strings.TrimSpace(ip)
				if ip != "" {
					out = append(out, ip)
				}
			}
		}
	}
	return out, nil
}

func (w *WGDashboardClient) downloadPeer(publicKey string) (string, error) {
	resp, err := w.req().Get(w.baseURL + "/api/downloadPeer/" + url.PathEscape(w.inboundID) + "?id=" + url.QueryEscape(publicKey))
	if err != nil {
		return "", err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return "", err
	}
	data := mapValueMap(raw, "data")
	return strings.TrimSpace(fmt.Sprintf("%v", data["file"])), nil
}

func (w *WGDashboardClient) saveRestrictJob(publicKey, field, value string) error {
	if strings.TrimSpace(publicKey) == "" || strings.TrimSpace(field) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	payload := map[string]interface{}{
		"Job": map[string]interface{}{
			"JobID":         randomWGID(12),
			"Configuration": w.inboundID,
			"Peer":          publicKey,
			"Field":         field,
			"Operator":      "lgt",
			"Value":         value,
			"CreationDate":  "",
			"ExpireDate":    nil,
			"Action":        "restrict",
		},
	}
	body, _ := json.Marshal(payload)
	_, err := w.req().Raw().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(w.baseURL + "/api/savePeerScheduleJob")
	return err
}

func (w *WGDashboardClient) allowAccess(publicKey string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"peers": []string{publicKey},
	})
	_, err := w.req().Raw().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(w.baseURL + "/api/allowAccessPeers/" + url.PathEscape(w.inboundID))
	return err
}

func (w *WGDashboardClient) restrictPeer(publicKey string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"peers": []string{publicKey},
	})
	_, err := w.req().Raw().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(w.baseURL + "/api/restrictPeers/" + url.PathEscape(w.inboundID))
	return err
}

type wgKeyPair struct {
	Private   string
	Public    string
	Preshared string
}

func newWGKeys() (*wgKeyPair, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return nil, err
	}
	// Clamp for Curve25519 private key.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, err
	}
	psk := make([]byte, 32)
	if _, err := rand.Read(psk); err != nil {
		return nil, err
	}
	return &wgKeyPair{
		Private:   base64.StdEncoding.EncodeToString(priv),
		Public:    base64.StdEncoding.EncodeToString(pub),
		Preshared: base64.StdEncoding.EncodeToString(psk),
	}, nil
}

func randomWGID(size int) string {
	if size <= 0 {
		size = 12
	}
	buf := make([]byte, size)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func toMapSlice(v interface{}) []map[string]interface{} {
	switch t := v.(type) {
	case []map[string]interface{}:
		return t
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func mapValueMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return map[string]interface{}{}
}

func jobValueFloat(jobs []map[string]interface{}, field string) float64 {
	for _, job := range jobs {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", job["Field"])), field) {
			return toFloat64(job["Value"])
		}
	}
	return 0
}

func jobValueString(jobs []map[string]interface{}, field string) string {
	for _, job := range jobs {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", job["Field"])), field) {
			return strings.TrimSpace(fmt.Sprintf("%v", job["Value"]))
		}
	}
	return ""
}

func parseWGDateJob(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "<nil>" {
		return 0
	}
	if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
		return t.Unix()
	}
	return parseAnyTime(raw)
}
