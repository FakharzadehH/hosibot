package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"hosibot/internal/models"
)

// LegacyHandler provides compatibility endpoints from legacy PHP files.
type LegacyHandler struct {
	repos        *Repos
	logger       *zap.Logger
	verifyBotKey string
}

func NewLegacyHandler(repos *Repos, logger *zap.Logger, verifyBotKey string) *LegacyHandler {
	return &LegacyHandler{
		repos:        repos,
		logger:       logger,
		verifyBotKey: strings.TrimSpace(verifyBotKey),
	}
}

// LogStats matches api/log.php output format.
func (h *LegacyHandler) LogStats(c echo.Context) error {
	countUser, countInvoice, countAgent := h.countStats()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"count_user":    countUser,
		"count_invoice": countInvoice,
		"count_agent":   countAgent,
	})
}

// StatBot matches api/statbot.php output format.
func (h *LegacyHandler) StatBot(c echo.Context) error {
	return h.LogStats(c)
}

// Keyboard matches api/keyboard.php output format.
func (h *LegacyHandler) Keyboard(c echo.Context) error {
	textMap := map[string]string{
		"text_usertest":           "",
		"text_Purchased_services": "",
		"text_support":            "",
		"text_help":               "",
		"accountwallet":           "",
		"text_sell":               "",
		"text_Tariff_list":        "",
		"text_affiliates":         "",
		"text_wheel_luck":         "",
		"text_extend":             "",
	}

	texts, _ := h.repos.Setting.GetAllTexts()
	for _, item := range texts {
		if _, ok := textMap[item.IDText]; ok {
			textMap[item.IDText] = item.Text
		}
	}

	var userList [][]map[string]interface{}
	if setting, err := h.repos.Setting.GetSettings(); err == nil && strings.TrimSpace(setting.KeyboardMain) != "" {
		var parsed struct {
			Keyboard [][]map[string]interface{} `json:"keyboard"`
		}
		if err := json.Unmarshal([]byte(setting.KeyboardMain), &parsed); err == nil {
			userList = parsed.Keyboard
		}
	}

	allKeys := []string{
		"text_sell",
		"text_extend",
		"text_usertest",
		"text_wheel_luck",
		"text_Purchased_services",
		"accountwallet",
		"text_affiliates",
		"text_Tariff_list",
		"text_support",
		"text_help",
	}
	used := make(map[string]bool, len(allKeys))
	for _, row := range userList {
		for _, btn := range row {
			txt, _ := btn["text"].(string)
			if txt != "" {
				used[txt] = true
			}
		}
	}

	keyList := make([][]map[string]string, 0)
	for _, key := range allKeys {
		if used[key] {
			continue
		}
		keyList = append(keyList, []map[string]string{{"text": key}})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"keylist":  keyList,
		"userlist": userList,
		"text":     textMap,
	})
}

// Verify matches api/verify.php token issue behavior for miniapp clients.
func (h *LegacyHandler) Verify(c echo.Context) error {
	if h.verifyBotKey == "" {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "Failed to generate session token",
			"token":  nil,
		})
	}

	rawBody, _ := io.ReadAll(c.Request().Body)
	_ = c.Request().Body.Close()

	candidates := collectInitDataCandidates(c, rawBody)
	if len(candidates) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"status": false,
			"msg":    "Telegram init data is missing or invalid",
			"token":  nil,
		})
	}

	var (
		userID    string
		verifyErr error
	)
	for _, candidate := range candidates {
		userID, verifyErr = validateTelegramInitData(candidate, h.verifyBotKey)
		if verifyErr == nil {
			break
		}
	}
	if verifyErr != nil {
		statusCode := http.StatusBadRequest
		msg := verifyErr.Error()
		if msg == "User verification failed" {
			statusCode = http.StatusForbidden
		}
		return c.JSON(statusCode, map[string]interface{}{
			"status": false,
			"msg":    msg,
			"token":  nil,
		})
	}

	if _, err := h.repos.User.FindByID(userID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"status": false,
			"msg":    "User not found",
			"token":  nil,
		})
	}

	tokenBytes := make([]byte, 20)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.logger.Error("Failed to generate verify token", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "Failed to generate session token",
			"token":  nil,
		})
	}
	sessionToken := hex.EncodeToString(tokenBytes)

	if err := h.repos.User.Update(userID, map[string]interface{}{
		"token": sql.NullString{String: sessionToken, Valid: true},
	}); err != nil {
		h.logger.Error("Failed to update user token", zap.Error(err), zap.String("user_id", userID))
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"status": false,
			"msg":    "Failed to generate session token",
			"token":  nil,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": true,
		"msg":    "User verified",
		"token":  sessionToken,
	})
}

func (h *LegacyHandler) countStats() (int64, int64, int64) {
	var countUser int64
	var countInvoice int64
	var countAgent int64
	db := h.repos.Setting.DB()
	_ = db.Model(&models.User{}).Count(&countUser).Error
	_ = db.Model(&models.Invoice{}).Count(&countInvoice).Error
	_ = db.Model(&models.User{}).Where("agent != ?", "f").Count(&countAgent).Error
	return countUser, countInvoice, countAgent
}

func collectInitDataCandidates(c echo.Context, rawBody []byte) []interface{} {
	candidates := make([]interface{}, 0, 10)
	add := func(v interface{}) {
		switch t := v.(type) {
		case nil:
			return
		case string:
			s := strings.TrimSpace(t)
			if s != "" {
				candidates = append(candidates, s)
			}
		case map[string]interface{}:
			if len(t) > 0 {
				candidates = append(candidates, t)
			}
		}
	}

	for _, key := range []string{
		"X-Telegram-Init-Data",
		"X-Telegram-Web-App-Init-Data",
		"Telegram-Init-Data",
	} {
		add(c.Request().Header.Get(key))
	}

	add(c.QueryParam("initData"))
	add(c.QueryParam("init_data"))
	add(c.FormValue("initData"))
	add(c.FormValue("init_data"))
	add(c.FormValue("initDataUnsafe"))
	add(c.FormValue("init_data_unsafe"))

	body := strings.TrimSpace(string(rawBody))
	if body == "" {
		return candidates
	}

	var parsed interface{}
	if err := json.Unmarshal(rawBody, &parsed); err == nil {
		switch t := parsed.(type) {
		case map[string]interface{}:
			add(t)
			add(t["initData"])
			add(t["init_data"])
			add(t["initDataUnsafe"])
			add(t["init_data_unsafe"])
		case string:
			add(t)
		}
	} else {
		add(body)
	}

	return candidates
}

func validateTelegramInitData(candidate interface{}, botToken string) (string, error) {
	initData, err := toInitDataMap(candidate)
	if err != nil {
		return "", err
	}

	hashRaw, ok := initData["hash"]
	if !ok {
		return "", fmt.Errorf("Telegram init data is missing required signature")
	}
	receivedHash, ok := hashRaw.(string)
	receivedHash = strings.TrimSpace(receivedHash)
	if !ok || receivedHash == "" {
		return "", fmt.Errorf("Telegram init data is missing required signature")
	}
	delete(initData, "hash")

	dataCheckArray := make([]string, 0, len(initData))
	for key, value := range initData {
		valueStr := normalizeInitValue(value)
		if valueStr == "" {
			continue
		}
		dataCheckArray = append(dataCheckArray, key+"="+valueStr)
	}
	if len(dataCheckArray) == 0 {
		return "", fmt.Errorf("Telegram init data payload is empty")
	}
	sort.Strings(dataCheckArray)
	dataCheckString := strings.Join(dataCheckArray, "\n")

	keyHMAC := hmac.New(sha256.New, []byte("WebAppData"))
	keyHMAC.Write([]byte(botToken))
	secretKey := keyHMAC.Sum(nil)

	dataHMAC := hmac.New(sha256.New, secretKey)
	dataHMAC.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(dataHMAC.Sum(nil))

	if !hmac.Equal([]byte(strings.ToLower(calculatedHash)), []byte(strings.ToLower(receivedHash))) {
		return "", fmt.Errorf("User verification failed")
	}

	userDataRaw, ok := initData["user"]
	if !ok {
		return "", fmt.Errorf("User data is missing or malformed in init data")
	}
	var userMap map[string]interface{}
	switch t := userDataRaw.(type) {
	case string:
		if err := json.Unmarshal([]byte(t), &userMap); err != nil {
			return "", fmt.Errorf("User data is missing or malformed in init data")
		}
	case map[string]interface{}:
		userMap = t
	default:
		return "", fmt.Errorf("User data is missing or malformed in init data")
	}
	userID := anyToString(userMap["id"])
	if userID == "" {
		return "", fmt.Errorf("User data is missing or malformed in init data")
	}
	return userID, nil
}

func toInitDataMap(candidate interface{}) (map[string]interface{}, error) {
	switch t := candidate.(type) {
	case map[string]interface{}:
		if len(t) == 0 {
			return nil, fmt.Errorf("Telegram init data payload is empty")
		}
		return t, nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil, fmt.Errorf("Telegram init data is missing or invalid")
		}
		if strings.HasPrefix(s, "{") {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(s), &m); err == nil && len(m) > 0 {
				return m, nil
			}
		}
		decoded := htmlEntityDecode(s)
		values, err := url.ParseQuery(decoded)
		if err != nil {
			return nil, fmt.Errorf("Telegram init data is missing or invalid")
		}
		m := make(map[string]interface{}, len(values))
		for k, v := range values {
			if len(v) == 0 {
				continue
			}
			m[k] = v[0]
		}
		if len(m) == 0 {
			return nil, fmt.Errorf("Telegram init data payload is empty")
		}
		return m, nil
	default:
		return nil, fmt.Errorf("Telegram init data is missing or invalid")
	}
}

func normalizeInitValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%.0f", t)
		}
		return fmt.Sprintf("%v", t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func anyToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	default:
		return ""
	}
}

func htmlEntityDecode(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&#38;", "&",
		"&equals;", "=",
		"&#61;", "=",
		"&plus;", "+",
		"&#43;", "+",
	)
	return replacer.Replace(s)
}
