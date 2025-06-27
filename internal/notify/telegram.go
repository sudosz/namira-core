package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"text/template"

	"github.com/NaMiraNet/namira-core/internal/core"
	"github.com/NaMiraNet/namira-core/internal/qr"
	"github.com/enescakir/emoji"
)

type Telegram struct {
	BotToken    string
	Channel     string
	Client      *http.Client
	Template    string
	qrGenerator *qr.QRGenerator
	mu          sync.RWMutex
	tmpl        *template.Template
}

func NewTelegram(botToken, channel, template, qrConfig string, client *http.Client) *Telegram {
	t := &Telegram{
		BotToken:    botToken,
		Channel:     channel,
		Template:    template,
		Client:      client,
		qrGenerator: qr.NewQRGenerator(qrConfig),
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
	funcMap := template.FuncMap{
		"protocolEmoji": func(protocol string) string {
			switch protocol {
			case "vmess":
				return emoji.HighVoltage.String()
			case "vless":
				return emoji.Rocket.String()
			case "trojan":
				return emoji.Shield.String()
			case "shadowsocks":
				return emoji.Locked.String()
			default:
				return emoji.RepeatButton.String()
			}
		},
		"countryFlag": func(countryCode string) string {
			e, err := emoji.CountryFlag(countryCode)
			if err != nil {
				return emoji.GlobeWithMeridians.String()
			}
			return e.String()
		},
	}
	if tmpl, err := template.New("telegram").Funcs(funcMap).Parse(t.Template); err == nil {
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

type telegramPhoto struct {
	ChatID    string `json:"chat_id"`
	Photo     string `json:"photo"`
	Caption   string `json:"caption,omitempty"`
	ParseMode string `json:"parse_mode,omitempty"`
}

func (t *Telegram) SendWithQRCode(result core.CheckResult) error {
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

	var caption bytes.Buffer
	if err := tmpl.Execute(&caption, result); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	jsonData, err := json.Marshal(telegramPhoto{
		ChatID:    t.Channel,
		Photo:     t.qrGenerator.GenerateURL(result.Raw),
		Caption:   caption.String(),
		ParseMode: "HTML",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.telegram.org/bot"+t.BotToken+"/sendPhoto",
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}
