package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"text/template"

	"github.com/NaMiraNet/namira-core/internal/core"
)

type Telegram struct {
	BotToken string
	Channel  string
	Client   *http.Client
	Template string
	mu       sync.RWMutex
	tmpl     *template.Template
}

func NewTelegram(botToken, channel, template string, client *http.Client) *Telegram {
	t := &Telegram{
		BotToken: botToken,
		Channel:  channel,
		Template: template,
		Client:   client,
	}
	t.initTemplate()
	return t
}

type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func (t *Telegram) initTemplate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tmpl, err := template.New("telegram").Parse(t.Template); err == nil {
		t.tmpl = tmpl
	}
}

func (t *Telegram) Send(result core.CheckResult) error {
	t.mu.RLock()
	tmpl := t.tmpl
	t.mu.RUnlock()

	if tmpl == nil {
		t.initTemplate()
		t.mu.RLock()
		tmpl = t.tmpl
		t.mu.RUnlock()
		if tmpl == nil {
			return fmt.Errorf("failed to initialize template")
		}
	}

	var message bytes.Buffer
	if err := tmpl.Execute(&message, result); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	jsonData, err := json.Marshal(telegramMessage{
		ChatID:    t.Channel,
		Text:      message.String(),
		ParseMode: "HTML",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, 
		"https://api.telegram.org/bot"+t.BotToken+"/sendMessage",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}
