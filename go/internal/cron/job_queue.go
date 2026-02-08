package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"hosibot/internal/models"
	"hosibot/internal/panel"
)

const (
	queueKindSendMessage = "sendmessage"
	queueKindGift        = "gift"

	sendMessageBatchSize = 20
	giftBatchSize        = 5
)

type sendMessageJobPayload struct {
	IDAdmin     string `json:"id_admin"`
	IDMessage   int    `json:"id_message"`
	Type        string `json:"type"`
	Message     string `json:"message"`
	PingMessage string `json:"pingmessage"`
	BtnMessage  string `json:"btnmessage"`
}

type giftJobPayload struct {
	IDAdmin   string `json:"id_admin"`
	IDMessage int    `json:"id_message"`
	NamePanel string `json:"name_panel"`
	TypeGift  string `json:"typegift"`
	Value     string `json:"value"`
	Text      string `json:"text"`
}

type telegramAPIResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

type telegramAPIError struct {
	description string
}

func (e *telegramAPIError) Error() string {
	if strings.TrimSpace(e.description) == "" {
		return "telegram api call failed"
	}
	return e.description
}

func (s *Scheduler) processQueuedJobs() {
	defer s.recoverFromPanic("processQueuedJobs")

	if s.repos == nil || s.repos.CronJob == nil {
		return
	}

	s.importLegacySendMessageJob()
	s.importLegacyGiftJob()
	s.processSendMessageJobs()
	s.processGiftJobs()
}

func (s *Scheduler) importLegacySendMessageJob() {
	infoPath, infoRaw, ok := readLegacyQueueJSON("info")
	if !ok {
		return
	}
	usersPath, usersRaw, ok := readLegacyQueueJSON("users.json")
	if !ok {
		return
	}

	payload, err := decodeLegacySendMessagePayload(infoRaw)
	if err != nil {
		s.logger.Warn("Invalid legacy sendmessage payload", zap.Error(err))
		return
	}
	targets := parseLegacyTargets(usersRaw, "id")

	externalRef := fmt.Sprintf("legacy-sendmessage:%s:%d:%s", payload.IDAdmin, payload.IDMessage, strings.ToLower(payload.Type))
	if _, err := s.repos.CronJob.CreateJobWithItems(queueKindSendMessage, externalRef, payload, targets); err != nil {
		s.logger.Error("Failed to import legacy sendmessage queue", zap.Error(err))
		return
	}

	_ = os.Remove(infoPath)
	_ = os.Remove(usersPath)
}

func (s *Scheduler) importLegacyGiftJob() {
	giftPath, giftRaw, ok := readLegacyQueueJSON("gift")
	if !ok {
		return
	}
	usernamesPath, usernamesRaw, ok := readLegacyQueueJSON("username.json")
	if !ok {
		return
	}

	payload, err := decodeLegacyGiftPayload(giftRaw)
	if err != nil {
		s.logger.Warn("Invalid legacy gift payload", zap.Error(err))
		return
	}
	targets := parseLegacyTargets(usernamesRaw, "username")

	externalRef := fmt.Sprintf("legacy-gift:%s:%d:%s:%s", payload.IDAdmin, payload.IDMessage, payload.NamePanel, strings.ToLower(payload.TypeGift))
	if _, err := s.repos.CronJob.CreateJobWithItems(queueKindGift, externalRef, payload, targets); err != nil {
		s.logger.Error("Failed to import legacy gift queue", zap.Error(err))
		return
	}

	_ = os.Remove(giftPath)
	_ = os.Remove(usernamesPath)
}

func (s *Scheduler) processSendMessageJobs() {
	job, err := s.repos.CronJob.FindNextActiveByKind(queueKindSendMessage)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		s.logger.Error("Failed to fetch sendmessage job", zap.Error(err))
		return
	}

	_ = s.repos.CronJob.MarkRunning(job.ID)

	var payload sendMessageJobPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		_ = s.repos.CronJob.Finalize(job.ID, "failed", trimErr("invalid payload: "+err.Error()))
		return
	}

	pendingCount, err := s.repos.CronJob.CountPendingItems(job.ID)
	if err != nil {
		s.logger.Error("Failed to count pending sendmessage items", zap.Uint("job_id", job.ID), zap.Error(err))
		return
	}
	if pendingCount == 0 {
		s.finalizeSendMessageJob(job.ID, payload)
		return
	}

	s.updateSendMessageProgress(payload, pendingCount)

	items, err := s.repos.CronJob.ListPendingItems(job.ID, sendMessageBatchSize)
	if err != nil {
		s.logger.Error("Failed to list sendmessage items", zap.Uint("job_id", job.ID), zap.Error(err))
		return
	}

	for _, item := range items {
		if err := s.executeSendMessageItem(payload, item.Target); err != nil {
			_ = s.repos.CronJob.MarkItemFailed(job.ID, item.ID, trimErr(err.Error()))
			continue
		}
		_ = s.repos.CronJob.MarkItemDone(job.ID, item.ID)
	}

	pendingAfter, err := s.repos.CronJob.CountPendingItems(job.ID)
	if err != nil {
		return
	}
	if pendingAfter == 0 {
		s.finalizeSendMessageJob(job.ID, payload)
	}
}

func (s *Scheduler) processGiftJobs() {
	job, err := s.repos.CronJob.FindNextActiveByKind(queueKindGift)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		s.logger.Error("Failed to fetch gift job", zap.Error(err))
		return
	}

	_ = s.repos.CronJob.MarkRunning(job.ID)

	var payload giftJobPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		_ = s.repos.CronJob.Finalize(job.ID, "failed", trimErr("invalid payload: "+err.Error()))
		return
	}

	panelModel, err := s.repos.Panel.FindByName(payload.NamePanel)
	if err != nil {
		_ = s.repos.CronJob.Finalize(job.ID, "failed", trimErr("panel not found: "+payload.NamePanel))
		return
	}

	panelClient, err := s.getPanelClient(panelModel)
	if err != nil {
		_ = s.repos.CronJob.Finalize(job.ID, "failed", trimErr(err.Error()))
		return
	}

	pendingCount, err := s.repos.CronJob.CountPendingItems(job.ID)
	if err != nil {
		return
	}
	if pendingCount == 0 {
		s.finalizeGiftJob(job.ID, payload)
		return
	}

	items, err := s.repos.CronJob.ListPendingItems(job.ID, giftBatchSize)
	if err != nil {
		return
	}

	for _, item := range items {
		if err := s.executeGiftItem(panelClient, payload, item.Target); err != nil {
			_ = s.repos.CronJob.MarkItemFailed(job.ID, item.ID, trimErr(err.Error()))
			continue
		}
		_ = s.repos.CronJob.MarkItemDone(job.ID, item.ID)
	}

	pendingAfter, err := s.repos.CronJob.CountPendingItems(job.ID)
	if err != nil {
		return
	}
	if pendingAfter == 0 {
		s.finalizeGiftJob(job.ID, payload)
	}
}

func (s *Scheduler) executeSendMessageItem(payload sendMessageJobPayload, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("empty user id")
	}

	jobType := strings.ToLower(strings.TrimSpace(payload.Type))
	switch jobType {
	case "unpinmessage":
		raw, err := s.botAPI.Call("unpinAllChatMessages", map[string]interface{}{
			"chat_id": userID,
		})
		_, err = parseTelegramAPIResponse(raw, err)
		return err

	case "forwardmessage":
		if strings.TrimSpace(payload.IDAdmin) == "" {
			return fmt.Errorf("forward message requires id_admin")
		}
		messageID := parseIntSafeCron(payload.Message)
		if messageID <= 0 {
			return fmt.Errorf("invalid forward message id")
		}

		raw, err := s.botAPI.ForwardMessage(userID, payload.IDAdmin, messageID)
		resp, err := parseTelegramAPIResponse(raw, err)
		if err != nil {
			return err
		}

		if shouldPinMessage(payload.PingMessage) && extractMessageID(resp.Result) > 0 {
			s.pinMessage(userID, extractMessageID(resp.Result))
		}
		return nil

	case "sendmessage", "xdaynotmessage":
		replyMarkup := s.sendMessageReplyMarkup(payload.BtnMessage)
		raw, err := s.botAPI.SendMessage(userID, payload.Message, replyMarkup)
		resp, err := parseTelegramAPIResponse(raw, err)
		if err != nil {
			if isBlockedByUser(err) {
				s.maybeDeleteBlockedUser(userID)
				return nil
			}
			return err
		}

		if shouldPinMessage(payload.PingMessage) && extractMessageID(resp.Result) > 0 {
			s.pinMessage(userID, extractMessageID(resp.Result))
		}
		return nil

	default:
		return fmt.Errorf("unsupported sendmessage type: %s", payload.Type)
	}
}

func (s *Scheduler) executeGiftItem(panelClient panel.PanelClient, payload giftJobPayload, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("empty username")
	}

	value := parseIntSafeCron(payload.Value)
	if value < 0 {
		value = 0
	}

	ctx := context.Background()
	panelUser, err := panelClient.GetUser(ctx, username)
	if err != nil {
		return fmt.Errorf("panel user lookup failed: %w", err)
	}

	invoice, err := s.repos.Invoice.FindByUsername(username)
	if err != nil {
		return fmt.Errorf("invoice not found for username %s: %w", username, err)
	}

	nowUnix := time.Now().Unix()
	typeGift := strings.ToLower(strings.TrimSpace(payload.TypeGift))
	serviceType := "gift_time"

	valuePayload := map[string]interface{}{
		"time_value": value,
		"old_volume": panelUser.DataLimit,
		"expire_old": panelUser.ExpireTime,
	}

	if typeGift == "volume" {
		newLimit := int64(0)
		if value != 0 {
			newLimit = panelUser.DataLimit + int64(value)*1024*1024*1024
		}

		_, err = panelClient.ModifyUser(ctx, username, panel.ModifyUserRequest{
			Status:    "active",
			DataLimit: newLimit,
		})
		if err != nil {
			return fmt.Errorf("gift volume failed: %w", err)
		}

		serviceType = "gift_volume"
		valuePayload = map[string]interface{}{
			"volume_value": value,
			"old_volume":   panelUser.DataLimit,
			"expire_old":   panelUser.ExpireTime,
		}
		s.resetInvoiceNotification(invoice, "volume")
	} else {
		baseExpire := panelUser.ExpireTime
		if baseExpire < nowUnix {
			baseExpire = nowUnix
		}

		newExpire := int64(0)
		if value != 0 {
			newExpire = baseExpire + int64(value)*86400
		}

		_, err = panelClient.ModifyUser(ctx, username, panel.ModifyUserRequest{
			Status:     "active",
			ExpireTime: newExpire,
		})
		if err != nil {
			return fmt.Errorf("gift time failed: %w", err)
		}

		serviceType = "gift_time"
		s.resetInvoiceNotification(invoice, "time")
	}

	_ = s.repos.Invoice.Update(invoice.IDInvoice, map[string]interface{}{
		"Status": "active",
	})

	if strings.TrimSpace(payload.Text) != "" {
		_, _ = s.botAPI.SendMessage(invoice.IDUser, payload.Text, nil)
	}

	valueRaw, _ := json.Marshal(valuePayload)
	outputRaw, _ := json.Marshal(map[string]interface{}{"status": true})
	service := &models.ServiceOther{
		IDUser:   invoice.IDUser,
		Username: invoice.Username,
		Value:    string(valueRaw),
		Type:     serviceType,
		Time:     time.Now().Format("2006/01/02 15:04:05"),
		Price:    "0",
		Output:   string(outputRaw),
	}
	if err := s.repos.Setting.DB().Create(service).Error; err != nil {
		return fmt.Errorf("failed to log gift in service_other: %w", err)
	}

	return nil
}

func (s *Scheduler) updateSendMessageProgress(payload sendMessageJobPayload, pendingCount int64) {
	if strings.TrimSpace(payload.IDAdmin) == "" || payload.IDMessage <= 0 {
		return
	}

	cancelMarkup := map[string]interface{}{
		"inline_keyboard": [][]map[string]string{
			{{"text": "لغو عملیات", "callback_data": "cancel_sendmessage"}},
		},
	}
	text := fmt.Sprintf("✏️ عملیات ارسال پیام درحال انجام می باشد...\n\nتعداد نفرات باقی مانده :  %d", pendingCount)
	_, _ = s.botAPI.EditMessageText(payload.IDAdmin, payload.IDMessage, text, cancelMarkup)
}

func (s *Scheduler) finalizeSendMessageJob(jobID uint, payload sendMessageJobPayload) {
	if strings.TrimSpace(payload.IDAdmin) != "" && payload.IDMessage > 0 {
		_, _ = s.botAPI.DeleteMessage(payload.IDAdmin, payload.IDMessage)
	}
	if strings.TrimSpace(payload.IDAdmin) != "" {
		_, _ = s.botAPI.SendMessage(payload.IDAdmin, "📌 عملیات برای تمامی کاربران درخواستی انجام شد.", nil)
	}
	_ = s.repos.CronJob.Finalize(jobID, "done", "")
}

func (s *Scheduler) finalizeGiftJob(jobID uint, payload giftJobPayload) {
	if strings.TrimSpace(payload.IDAdmin) != "" && payload.IDMessage > 0 {
		_, _ = s.botAPI.DeleteMessage(payload.IDAdmin, payload.IDMessage)
	}
	if strings.TrimSpace(payload.IDAdmin) != "" {
		_, _ = s.botAPI.SendMessage(payload.IDAdmin, "📌 عملیات برای تمامی سرویس های درخواستی انجام شد.", nil)
	}
	_ = s.repos.CronJob.Finalize(jobID, "done", "")
}

func (s *Scheduler) maybeDeleteBlockedUser(userID string) {
	invoiceCount, err := s.repos.Invoice.CountByUserID(userID)
	if err != nil || invoiceCount != 0 {
		return
	}

	user, err := s.repos.User.FindByID(userID)
	if err != nil || user == nil {
		return
	}
	if user.Balance != 0 {
		return
	}
	_ = s.repos.User.Delete(userID)
}

func (s *Scheduler) pinMessage(chatID string, messageID int) {
	raw, err := s.botAPI.Call("pinChatMessage", map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	})
	_, _ = parseTelegramAPIResponse(raw, err)
}

func (s *Scheduler) sendMessageReplyMarkup(btnType string) interface{} {
	switch strings.ToLower(strings.TrimSpace(btnType)) {
	case "", "none":
		return nil
	case "buy":
		return buildSingleCallbackButton(s.mustText("text_sell", "🛒 خرید سرویس"), "buy")
	case "start":
		return buildSingleCallbackButton("شروع", "start")
	case "usertestbtn":
		return buildSingleCallbackButton(s.mustText("text_usertest", "🧪 سرویس تست"), "usertestbtn")
	case "helpbtn":
		return buildSingleCallbackButton(s.mustText("text_help", "📚 راهنما"), "helpbtn")
	case "affiliatesbtn":
		return buildSingleCallbackButton(s.mustText("text_affiliates", "📢 معرفی به دوستان"), "affiliatesbtn")
	case "addbalance", "add_balance":
		return buildSingleCallbackButton(s.mustText("text_Add_Balance", "💰 شارژ کیف پول"), "Add_Balance")
	default:
		return nil
	}
}

func (s *Scheduler) mustText(key, fallback string) string {
	text, err := s.repos.Setting.GetText(key)
	if err != nil || strings.TrimSpace(text) == "" {
		return fallback
	}
	return text
}

func (s *Scheduler) resetInvoiceNotification(invoice *models.Invoice, key string) {
	notifs := map[string]bool{
		"volume": false,
		"time":   false,
	}
	if strings.TrimSpace(invoice.Notifications) != "" {
		var existing map[string]bool
		if err := json.Unmarshal([]byte(invoice.Notifications), &existing); err == nil {
			if v, ok := existing["volume"]; ok {
				notifs["volume"] = v
			}
			if v, ok := existing["time"]; ok {
				notifs["time"] = v
			}
		}
	}
	notifs[key] = false
	raw, _ := json.Marshal(notifs)
	_ = s.repos.Invoice.Update(invoice.IDInvoice, map[string]interface{}{
		"notifctions": string(raw),
	})
}

func decodeLegacySendMessagePayload(raw []byte) (sendMessageJobPayload, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return sendMessageJobPayload{}, err
	}

	payload := sendMessageJobPayload{
		IDAdmin:     strings.TrimSpace(parseStringSafe(data["id_admin"])),
		IDMessage:   parseIntAny(data["id_message"]),
		Type:        strings.TrimSpace(parseStringSafe(data["type"])),
		Message:     strings.TrimSpace(parseStringSafe(data["message"])),
		PingMessage: strings.TrimSpace(parseStringSafe(data["pingmessage"])),
		BtnMessage:  strings.TrimSpace(parseStringSafe(data["btnmessage"])),
	}
	if payload.Type == "" {
		return sendMessageJobPayload{}, fmt.Errorf("missing type")
	}
	return payload, nil
}

func decodeLegacyGiftPayload(raw []byte) (giftJobPayload, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return giftJobPayload{}, err
	}

	payload := giftJobPayload{
		IDAdmin:   strings.TrimSpace(parseStringSafe(data["id_admin"])),
		IDMessage: parseIntAny(data["id_message"]),
		NamePanel: strings.TrimSpace(parseStringSafe(data["name_panel"])),
		TypeGift:  strings.TrimSpace(parseStringSafe(data["typegift"])),
		Value:     strings.TrimSpace(parseStringSafe(data["value"])),
		Text:      strings.TrimSpace(parseStringSafe(data["text"])),
	}
	if payload.NamePanel == "" {
		return giftJobPayload{}, fmt.Errorf("missing name_panel")
	}
	return payload, nil
}

func parseLegacyTargets(raw []byte, field string) []string {
	var listAny []interface{}
	if err := json.Unmarshal(raw, &listAny); err != nil {
		return nil
	}

	out := make([]string, 0, len(listAny))
	for _, item := range listAny {
		switch v := item.(type) {
		case map[string]interface{}:
			target := strings.TrimSpace(parseStringSafe(v[field]))
			if target == "" && field == "id" {
				target = strings.TrimSpace(parseStringSafe(v["user_id"]))
			}
			if target != "" {
				out = append(out, target)
			}
		case string:
			target := strings.TrimSpace(v)
			if target != "" {
				out = append(out, target)
			}
		case float64:
			out = append(out, strconv.FormatInt(int64(v), 10))
		}
	}
	return out
}

func readLegacyQueueJSON(filename string) (string, []byte, bool) {
	for _, p := range legacyQueueCandidates(filename) {
		raw, err := os.ReadFile(p)
		if err == nil {
			return p, raw, true
		}
	}
	return "", nil, false
}

func legacyQueueCandidates(filename string) []string {
	return []string{
		filepath.Join("cronbot", filename),
		filepath.Join("..", "cronbot", filename),
		filepath.Join("..", "..", "cronbot", filename),
	}
}

func parseTelegramAPIResponse(raw string, callErr error) (*telegramAPIResponse, error) {
	if callErr != nil {
		return nil, callErr
	}

	var parsed telegramAPIResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	if !parsed.OK {
		return &parsed, &telegramAPIError{description: parsed.Description}
	}
	return &parsed, nil
}

func extractMessageID(raw json.RawMessage) int {
	var resultMap map[string]interface{}
	if err := json.Unmarshal(raw, &resultMap); err != nil {
		return 0
	}
	return parseIntAny(resultMap["message_id"])
}

func shouldPinMessage(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	return v == "yes" || v == "1" || v == "on" || v == "true"
}

func buildSingleCallbackButton(text, callbackData string) map[string]interface{} {
	return map[string]interface{}{
		"inline_keyboard": [][]map[string]string{
			{{"text": text, "callback_data": callbackData}},
		},
	}
}

func isBlockedByUser(err error) bool {
	var tgErr *telegramAPIError
	if errors.As(err, &tgErr) {
		msg := strings.ToLower(strings.TrimSpace(tgErr.description))
		return strings.Contains(msg, "bot was blocked by the user")
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "bot was blocked by the user")
}

func parseIntAny(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		n, _ := strconv.Atoi(strings.TrimSpace(parseStringSafe(v)))
		return n
	}
}

func trimErr(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 900 {
		msg = msg[:900]
	}
	return msg
}
