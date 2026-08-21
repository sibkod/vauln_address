package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// TelegramService forwards drainer reports to the team chat via a bot.
// When the bot token or chat ID are not configured, Send reports it
// explicitly so callers can mark the delivery as skipped.
type TelegramService struct {
	botToken string
	chatID   string
	client   *http.Client
}

var ErrTelegramNotConfigured = errors.New("telegram bot is not configured")

func NewTelegramService(botToken, chatID string) *TelegramService {
	return &TelegramService{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *TelegramService) Configured() bool {
	return s.botToken != "" && s.chatID != ""
}

// Send posts a message to the configured chat through the Bot API.
func (s *TelegramService) Send(ctx context.Context, text string) error {
	if !s.Configured() {
		return ErrTelegramNotConfigured
	}

	payload := map[string]interface{}{
		"chat_id":                  s.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return fmt.Errorf("telegram API returned %d: %s", resp.StatusCode, apiErr.Description)
	}
	return nil
}
