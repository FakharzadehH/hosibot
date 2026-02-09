package panel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hosibot/internal/pkg/httpclient"
)

type IBSngClient struct {
	baseURL      string
	username     string
	password     string
	defaultGroup string
	client       *httpclient.Client
	authedAt     time.Time
}

func NewIBSngClient(baseURL, username, password, defaultGroup string) PanelClient {
	baseURL = strings.TrimSpace(baseURL)
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	c := httpclient.New().WithTimeout(20 * time.Second).WithInsecureSkipVerify()
	jar, _ := cookiejar.New(nil)
	c.Raw().SetCookieJar(jar)

	return &IBSngClient{
		baseURL:      baseURL,
		username:     strings.TrimSpace(username),
		password:     password,
		defaultGroup: strings.TrimSpace(defaultGroup),
		client:       c,
	}
}

func (i *IBSngClient) PanelType() string { return "ibsng" }

func (i *IBSngClient) Authenticate(ctx context.Context) error {
	if !i.authedAt.IsZero() && time.Since(i.authedAt) < 15*time.Minute {
		return nil
	}
	resp, err := i.client.Raw().R().
		SetFormData(map[string]string{
			"username": i.username,
			"password": i.password,
		}).
		Post(i.baseURL + "/IBSng/admin/")
	if err != nil {
		return fmt.Errorf("ibsng login failed: %w", err)
	}
	body := string(resp.Body())
	if !strings.Contains(body, "admin_index") && !strings.Contains(body, "IBSng/admin/") {
		return fmt.Errorf("ibsng login rejected")
	}
	i.authedAt = time.Now()
	return nil
}

func (i *IBSngClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	if err := i.Authenticate(ctx); err != nil {
		return nil, err
	}
	output, err := i.loadUserPage(username)
	if err != nil {
		return nil, err
	}

	statusRaw := extractBetween(output, "Status", "</td>")
	status := "active"
	if strings.Contains(strings.ToLower(statusRaw), "disable") || strings.Contains(strings.ToLower(statusRaw), "inactive") {
		status = "disabled"
	}

	dataLimitBytes := parseTrafficBytes(extractNthTrafficCell(output, true))
	usedTrafficBytes := parseTrafficBytes(extractNthTrafficCell(output, false))

	expire := int64(0)
	expDate := extractAfter(output, "Nearest Expiration Date:")
	expDate = strings.TrimSpace(stripTags(expDate))
	if expDate != "" && expDate != "---------------" {
		expire = parseAnyTime(expDate)
	}

	return &PanelUser{
		Username:    username,
		Status:      status,
		DataLimit:   dataLimitBytes,
		UsedTraffic: usedTrafficBytes,
		ExpireTime:  expire,
		Links:       []string{},
	}, nil
}

func (i *IBSngClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	if err := i.Authenticate(ctx); err != nil {
		return nil, err
	}

	group := i.defaultGroup
	if g := pickProfile(req.Inbounds); g != "" {
		group = g
	}
	if group == "" {
		group = "default"
	}

	uid, err := i.createUID(group, "1")
	if err != nil {
		return nil, err
	}

	password := ibsngRandomHex(6)
	ownerName := "system"
	editURL := fmt.Sprintf(
		"%s/IBSng/admin/plugins/edit.php?edit_user=1&user_id=%s&submit_form=1&add=1&count=1&credit=1&owner_name=%s&group_name=%s&x=35&y=1&edit__normal_username=normal_username",
		i.baseURL,
		url.QueryEscape(uid),
		url.QueryEscape(ownerName),
		url.QueryEscape(group),
	)

	form := map[string]string{
		"target":                  "user",
		"target_id":               uid,
		"update":                  "1",
		"edit_tpl_cs":             "normal_username",
		"attr_update_method_0":    "normalAttrs",
		"has_normal_username":     "t",
		"current_normal_username": "",
		"normal_username":         req.Username,
		"password":                password,
		"normal_save_user_add":    "t",
		"credit":                  "1",
	}
	resp, err := i.client.Raw().R().SetFormData(form).Post(editURL)
	if err != nil {
		return nil, fmt.Errorf("ibsng add user failed: %w", err)
	}
	body := string(resp.Body())
	if strings.Contains(strings.ToLower(body), "exist") {
		return nil, fmt.Errorf("username already exists")
	}
	if !strings.Contains(body, "user_info.php?user_id_multi") {
		return nil, fmt.Errorf("ibsng add user rejected")
	}

	return &PanelUser{
		Username: req.Username,
		Status:   "active",
		SubLink:  password,
		Links:    []string{},
	}, nil
}

func (i *IBSngClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	// IBSng admin panel update flow is highly form-specific and not currently needed for critical flows.
	return i.GetUser(ctx, username)
}

func (i *IBSngClient) DeleteUser(ctx context.Context, username string) error {
	if err := i.Authenticate(ctx); err != nil {
		return err
	}
	uid, err := i.findUID(username)
	if err != nil {
		return err
	}
	form := map[string]string{
		"user_id":                uid,
		"delete":                 "1",
		"delete_comment":         "",
		"delete_connection_logs": "on",
		"delete_audit_logs":      "on",
	}
	resp, err := i.client.Raw().R().SetFormData(form).Post(i.baseURL + "/IBSng/admin/user/del_user.php")
	if err != nil {
		return err
	}
	if !strings.Contains(string(resp.Body()), "Successfully") {
		return fmt.Errorf("ibsng delete failed")
	}
	return nil
}

func (i *IBSngClient) EnableUser(ctx context.Context, username string) error {
	return nil
}

func (i *IBSngClient) DisableUser(ctx context.Context, username string) error {
	return nil
}

func (i *IBSngClient) ResetTraffic(ctx context.Context, username string) error {
	return nil
}

func (i *IBSngClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	return "", nil
}

func (i *IBSngClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	if strings.TrimSpace(i.defaultGroup) == "" {
		return []PanelInbound{}, nil
	}
	return []PanelInbound{{
		Tag:      i.defaultGroup,
		Protocol: "group",
		Remark:   "ibsng group",
	}}, nil
}

func (i *IBSngClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (i *IBSngClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	return "", nil
}

func (i *IBSngClient) loadUserPage(username string) (string, error) {
	resp, err := i.client.Raw().R().
		Get(i.baseURL + "/IBSng/admin/user/user_info.php?normal_username_multi=" + url.QueryEscape(username))
	if err != nil {
		return "", err
	}
	body := string(resp.Body())
	if strings.Contains(body, "does not exists") {
		return "", fmt.Errorf("user not found")
	}
	return body, nil
}

func (i *IBSngClient) findUID(username string) (string, error) {
	body, err := i.loadUserPage(username)
	if err != nil {
		return "", err
	}
	pat := "change_credit.php?user_id="
	idx := strings.Index(body, pat)
	if idx < 0 {
		return "", fmt.Errorf("uid not found")
	}
	rest := body[idx+len(pat):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return "", fmt.Errorf("uid not found")
	}
	uid := strings.TrimSpace(rest[:end])
	if uid == "" {
		return "", fmt.Errorf("uid not found")
	}
	return uid, nil
}

func (i *IBSngClient) createUID(groupName, credit string) (string, error) {
	form := map[string]string{
		"submit_form":           "1",
		"add":                   "1",
		"count":                 "1",
		"credit":                credit,
		"owner_name":            i.username,
		"group_name":            groupName,
		"edit__normal_username": "1",
	}
	resp, err := i.client.Raw().R().
		SetFormData(form).
		Post(i.baseURL + "/IBSng/admin/user/add_new_users.php")
	if err != nil {
		return "", err
	}
	body := string(resp.Body())
	pat := "user_id="
	idx := strings.Index(body, pat)
	if idx < 0 {
		return "", fmt.Errorf("failed to retrieve uid")
	}
	rest := body[idx+len(pat):]
	end := strings.Index(rest, "&")
	if end < 0 {
		end = strings.Index(rest, "\"")
	}
	if end < 0 {
		return "", fmt.Errorf("failed to retrieve uid")
	}
	uid := strings.TrimSpace(rest[:end])
	if uid == "" {
		return "", fmt.Errorf("failed to retrieve uid")
	}
	return uid, nil
}

func extractNthTrafficCell(html string, first bool) string {
	pattern := `<td class="Form_Content_Row_Left_userinfo_light"><nobr>\s*Traffic Limit`
	re := regexp.MustCompile(pattern)
	idx := re.FindStringIndex(html)
	if idx == nil {
		return ""
	}
	segment := html[idx[0]:]
	cellClass := `<td class="Form_Content_Row_Right_userinfo_dark">`
	if first {
		cellClass = `<td class="Form_Content_Row_Right_userinfo_light">`
	}
	return extractBetween(segment, cellClass, "</td>")
}

func parseTrafficBytes(raw string) int64 {
	raw = strings.TrimSpace(stripTags(raw))
	if raw == "" || raw == "---------------" {
		return 0
	}
	raw = strings.ReplaceAll(raw, ",", "")
	re := regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*([GMK]?)`)
	m := re.FindStringSubmatch(strings.ToUpper(raw))
	if len(m) < 3 {
		v, _ := strconv.ParseFloat(raw, 64)
		return int64(v)
	}
	num, _ := strconv.ParseFloat(m[1], 64)
	unit := m[2]
	switch unit {
	case "G":
		return int64(num * float64(1024*1024*1024))
	case "M":
		return int64(num * float64(1024*1024))
	case "K":
		return int64(num * 1024)
	default:
		return int64(num)
	}
}

func extractAfter(s, marker string) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return ""
	}
	return s[idx+len(marker):]
}

func extractBetween(s, left, right string) string {
	l := strings.Index(s, left)
	if l < 0 {
		return ""
	}
	rest := s[l+len(left):]
	r := strings.Index(rest, right)
	if r < 0 {
		return rest
	}
	return rest[:r]
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}

func ibsngRandomHex(size int) string {
	if size <= 0 {
		size = 6
	}
	buf := make([]byte, size)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
