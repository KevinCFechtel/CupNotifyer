package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

const telegramAPIBaseURL = "https://api.telegram.org"

type Notifier struct {
	botToken   string
	chatID     string
	httpClient *http.Client
	apiBaseURL string
}

type sendMessageRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func New(botToken, chatID string, httpClient *http.Client) *Notifier {
	return &Notifier{
		botToken:   botToken,
		chatID:     chatID,
		httpClient: httpClient,
		apiBaseURL: telegramAPIBaseURL,
	}
}

func (n *Notifier) ID() string {
	return "telegram:" + n.chatID
}

func (n *Notifier) Send(ctx context.Context, available update.Available) error {
	payload, err := json.Marshal(sendMessageRequest{
		ChatID:                n.chatID,
		Text:                  formatMessage(available),
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
	if err != nil {
		return fmt.Errorf("encode Telegram message: %w", err)
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(n.apiBaseURL, "/"), n.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create Telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "CupNotifyer/1")

	res, err := n.httpClient.Do(req)
	if err != nil {
		sanitizedError := strings.ReplaceAll(err.Error(), n.botToken, "<redacted>")
		return fmt.Errorf("send Telegram message: %s", sanitizedError)
	}
	defer res.Body.Close()

	var response apiResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); err != nil {
		return fmt.Errorf("decode Telegram response (%s): %w", res.Status, err)
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices || !response.OK {
		return fmt.Errorf("Telegram API returned %s: %s", res.Status, response.Description)
	}
	return nil
}

func formatMessage(available update.Available) string {
	var message strings.Builder
	message.WriteString("📦 <b>Container-Update verfügbar</b>\n")
	message.WriteString("<b>Image:</b> <code>")
	message.WriteString(html.EscapeString(available.Reference))
	message.WriteString("</code>\n")

	if available.Server != "" {
		message.WriteString("<b>Server:</b> ")
		message.WriteString(html.EscapeString(available.Server))
		message.WriteString("\n")
	}

	switch available.Kind {
	case update.KindVersion:
		if available.CurrentVersion != "" {
			message.WriteString("<b>Aktuell:</b> ")
			message.WriteString(html.EscapeString(available.CurrentVersion))
			message.WriteString("\n")
		}
		message.WriteString("<b>Neu:</b> ")
		if available.NewVersion != "" {
			message.WriteString(html.EscapeString(available.NewVersion))
			message.WriteString(" (")
			message.WriteString(html.EscapeString(available.NewTag))
			message.WriteString(")")
		} else {
			message.WriteString(html.EscapeString(available.NewTag))
		}
		message.WriteString("\n")
		if available.UpdateType != "" {
			message.WriteString("<b>Typ:</b> ")
			message.WriteString(html.EscapeString(available.UpdateType))
			message.WriteString("\n")
		}
	case update.KindDigest:
		if len(available.LocalDigests) > 0 {
			message.WriteString("<b>Aktueller Digest:</b> <code>")
			message.WriteString(html.EscapeString(shortDigest(available.LocalDigests[0])))
			message.WriteString("</code>\n")
		}
		message.WriteString("<b>Neuer Digest:</b> <code>")
		message.WriteString(html.EscapeString(shortDigest(available.RemoteDigest)))
		message.WriteString("</code>\n")
	}

	if projectURL := safeProjectURL(available.URL); projectURL != "" {
		message.WriteString("<a href=\"")
		message.WriteString(html.EscapeString(projectURL))
		message.WriteString("\">Projekt öffnen</a>")
	}
	return strings.TrimSpace(message.String())
}

func safeProjectURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

func shortDigest(digest string) string {
	const visibleHashLength = 12
	prefix, hash, found := strings.Cut(digest, ":")
	if !found || len(hash) <= visibleHashLength {
		return digest
	}
	return prefix + ":" + hash[:visibleHashLength]
}
