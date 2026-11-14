package pushover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/KevinCFechtel/CupNotifyer/internal/notification"
	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

const (
	pushoverAPIURL  = "https://api.pushover.net/1/messages.json"
	maxMessageRunes = 1024
	updateTitle     = "Container-Update verfügbar"
	startupTitle    = "CupNotifyer gestartet"
)

type Notifier struct {
	apiToken   string
	userKey    string
	targetID   string
	httpClient *http.Client
	apiURL     string
}

type apiResponse struct {
	Status  int      `json:"status"`
	Request string   `json:"request"`
	Errors  []string `json:"errors"`
}

func New(apiToken, userKey string, httpClient *http.Client) *Notifier {
	userHash := sha256.Sum256([]byte(userKey))
	return &Notifier{
		apiToken:   apiToken,
		userKey:    userKey,
		targetID:   "pushover:" + hex.EncodeToString(userHash[:8]),
		httpClient: httpClient,
		apiURL:     pushoverAPIURL,
	}
}

func (n *Notifier) ID() string {
	return n.targetID
}

func (n *Notifier) SendStartup(ctx context.Context, startup notification.Startup) error {
	message := fmt.Sprintf(
		"Der Service wurde gestartet und prüft Cup alle %s auf Image-Updates.",
		startup.PollInterval,
	)
	return n.send(ctx, startupTitle, message, "")
}

func (n *Notifier) Send(ctx context.Context, available update.Available) error {
	return n.send(ctx, updateTitle, formatMessage(available), safeProjectURL(available.URL))
}

func (n *Notifier) send(ctx context.Context, title, message, projectURL string) error {
	form := url.Values{
		"token":   {n.apiToken},
		"user":    {n.userKey},
		"title":   {title},
		"message": {truncateMessage(message)},
	}
	if projectURL != "" {
		form.Set("url", projectURL)
		form.Set("url_title", "Projekt öffnen")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		n.apiURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("create Pushover request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "CupNotifyer/1")

	res, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send Pushover notification: %w", err)
	}
	defer res.Body.Close()

	var response apiResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); err != nil {
		return fmt.Errorf("decode Pushover response (%s): %w", res.Status, err)
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices || response.Status != 1 {
		details := strings.Join(response.Errors, "; ")
		if details == "" {
			details = "request was rejected"
		}
		return fmt.Errorf("Pushover API returned %s: %s", res.Status, details)
	}
	return nil
}

func formatMessage(available update.Available) string {
	var message strings.Builder
	message.WriteString("Image: ")
	message.WriteString(available.Reference)
	message.WriteString("\n")

	if available.Server != "" {
		message.WriteString("Server: ")
		message.WriteString(available.Server)
		message.WriteString("\n")
	}

	switch available.Kind {
	case update.KindVersion:
		if available.CurrentVersion != "" {
			message.WriteString("Aktuell: ")
			message.WriteString(available.CurrentVersion)
			message.WriteString("\n")
		}
		message.WriteString("Neu: ")
		if available.NewVersion != "" {
			message.WriteString(available.NewVersion)
			message.WriteString(" (")
			message.WriteString(available.NewTag)
			message.WriteString(")")
		} else {
			message.WriteString(available.NewTag)
		}
		message.WriteString("\n")
		if available.UpdateType != "" {
			message.WriteString("Typ: ")
			message.WriteString(available.UpdateType)
			message.WriteString("\n")
		}
	case update.KindDigest:
		if len(available.LocalDigests) > 0 {
			message.WriteString("Aktueller Digest: ")
			message.WriteString(shortDigest(available.LocalDigests[0]))
			message.WriteString("\n")
		}
		message.WriteString("Neuer Digest: ")
		message.WriteString(shortDigest(available.RemoteDigest))
		message.WriteString("\n")
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

func truncateMessage(message string) string {
	runes := []rune(message)
	if len(runes) <= maxMessageRunes {
		return message
	}
	return string(runes[:maxMessageRunes-1]) + "…"
}
