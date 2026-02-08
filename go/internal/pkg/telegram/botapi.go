package telegram

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
)

// BotAPI provides a direct Telegram Bot API client.
// Used for methods not covered by telebot, matching PHP's botapi.php behavior.
type BotAPI struct {
	token  string
	client *resty.Client
}

// NewBotAPI creates a new direct Telegram Bot API client.
func NewBotAPI(token string) *BotAPI {
	return &BotAPI{
		token:  token,
		client: resty.New().SetBaseURL("https://api.telegram.org/bot" + token),
	}
}

// Call makes a raw API call to the Telegram Bot API.
func (b *BotAPI) Call(method string, params map[string]interface{}) (string, error) {
	resp, err := b.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(params).
		Post("/" + method)
	if err != nil {
		return "", fmt.Errorf("telegram API call %s failed: %w", method, err)
	}
	return resp.String(), nil
}

// SendMessage sends a text message.
func (b *BotAPI) SendMessage(chatID string, text string, replyMarkup interface{}) (string, error) {
	params := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if replyMarkup != nil {
		params["reply_markup"] = replyMarkup
	}
	return b.Call("sendMessage", params)
}

// EditMessageText edits a message's text.
func (b *BotAPI) EditMessageText(chatID string, messageID int, text string, replyMarkup interface{}) (string, error) {
	params := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if replyMarkup != nil {
		params["reply_markup"] = replyMarkup
	}
	return b.Call("editMessageText", params)
}

// DeleteMessage deletes a message.
func (b *BotAPI) DeleteMessage(chatID string, messageID int) (string, error) {
	return b.Call("deleteMessage", map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	})
}

// ForwardMessage forwards a message.
func (b *BotAPI) ForwardMessage(chatID, fromChatID string, messageID int) (string, error) {
	return b.Call("forwardMessage", map[string]interface{}{
		"chat_id":      chatID,
		"from_chat_id": fromChatID,
		"message_id":   messageID,
	})
}

// GetChat gets chat information.
func (b *BotAPI) GetChat(chatID string) (string, error) {
	return b.Call("getChat", map[string]interface{}{
		"chat_id": chatID,
	})
}

// GetChatMember gets a chat member's status.
func (b *BotAPI) GetChatMember(chatID, userID string) (string, error) {
	return b.Call("getChatMember", map[string]interface{}{
		"chat_id": chatID,
		"user_id": userID,
	})
}

// AnswerCallbackQuery answers an inline callback query.
func (b *BotAPI) AnswerCallbackQuery(callbackQueryID, text string, showAlert bool) (string, error) {
	return b.Call("answerCallbackQuery", map[string]interface{}{
		"callback_query_id": callbackQueryID,
		"text":              text,
		"show_alert":        showAlert,
	})
}

// SendDocument sends a document.
func (b *BotAPI) SendDocument(chatID string, fileData []byte, filename, caption string) (string, error) {
	resp, err := b.client.R().
		SetFileReader("document", filename, io.NopCloser(strings.NewReader(string(fileData)))).
		SetFormData(map[string]string{
			"chat_id":    chatID,
			"caption":    caption,
			"parse_mode": "HTML",
		}).
		Post("/sendDocument")
	if err != nil {
		return "", err
	}
	return resp.String(), nil
}

// SendPhoto sends a photo (supports base64 or file_id).
func (b *BotAPI) SendPhoto(chatID, photo, caption string) (string, error) {
	params := map[string]interface{}{
		"chat_id":    chatID,
		"photo":      photo,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	return b.Call("sendPhoto", params)
}

// SendPhotoBase64 sends a photo from base64 encoded data.
func (b *BotAPI) SendPhotoBase64(chatID string, data string, caption string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}
	resp, err := b.client.R().
		SetFileReader("photo", "photo.jpg", io.NopCloser(strings.NewReader(string(decoded)))).
		SetFormData(map[string]string{
			"chat_id":    chatID,
			"caption":    caption,
			"parse_mode": "HTML",
		}).
		Post("/sendPhoto")
	if err != nil {
		return "", err
	}
	return resp.String(), nil
}

// SetWebhook sets the webhook URL.
func (b *BotAPI) SetWebhook(url string) (string, error) {
	return b.Call("setWebhook", map[string]interface{}{
		"url": url,
	})
}

// CheckTelegramIP verifies the request originates from Telegram's IP range.
// Matches PHP's checktelegramip() function.
func CheckTelegramIP(ip string) bool {
	// Telegram Bot API sends webhooks from these IP ranges:
	// 149.154.160.0/20 and 91.108.4.0/22
	allowedRanges := []string{
		"149.154.", "91.108.",
	}
	for _, prefix := range allowedRanges {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}
	return false
}

// DownloadFile downloads a file from Telegram's servers.
func (b *BotAPI) DownloadFile(filePath string) ([]byte, error) {
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, filePath)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
