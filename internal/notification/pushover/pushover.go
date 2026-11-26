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

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

const (
	pushoverAPIURL  = "https://api.pushover.net/1/messages.json"
	maxTitleRunes   = 250
	maxMessageRunes = 1024
	maxURLRunes     = 512
	updateTitle     = "Container-Update verfügbar"
)

type Notifier struct {
	apiToken      string
	userKey       string
	targetID      string
	httpClient    *http.Client
	apiURL        string
	encryptionKey []byte
}

type apiResponse struct {
	Status  int      `json:"status"`
	Request string   `json:"request"`
	Errors  []string `json:"errors"`
}

func New(apiToken, userKey, encryptionKey string, httpClient *http.Client) (*Notifier, error) {
	key, err := parseEncryptionKey(encryptionKey)
	if err != nil {
		return nil, err
	}
	userHash := sha256.Sum256([]byte(userKey))
	return &Notifier{
		apiToken:      apiToken,
		userKey:       userKey,
		targetID:      "pushover:" + hex.EncodeToString(userHash[:8]),
		httpClient:    httpClient,
		apiURL:        pushoverAPIURL,
		encryptionKey: key,
	}, nil
}

func (n *Notifier) ID() string {
	return n.targetID
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
	if n.encryptionKey != nil {
		// Even an empty GZIP/AES/HMAC envelope exceeds the 100-character
		// url_title limit. Let clients display the decrypted URL instead.
		form.Del("url_title")
		for _, field := range []struct {
			name  string
			limit int
		}{
			{"title", maxTitleRunes},
			{"message", maxMessageRunes},
			{"url", maxURLRunes},
		} {
			if !form.Has(field.name) {
				continue
			}
			plaintext := form.Get(field.name)
			var encrypted string
			var err error
			if field.name == "url" {
				// Shortening a URL would silently change or break its destination.
				encrypted, err = encryptExactField(plaintext, n.encryptionKey, field.limit)
			} else {
				encrypted, err = encryptField(truncateField(plaintext, field.limit), n.encryptionKey, field.limit)
			}
			if err != nil {
				return fmt.Errorf("encrypt Pushover %s: %w", field.name, err)
			}
			form.Set(field.name, encrypted)
		}
		form.Set("encrypted", "1")
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
	return truncateField(message, maxMessageRunes)
}

func truncateField(message string, limit int) string {
	runes := []rune(message)
	if len(runes) <= limit {
		return message
	}
	return string(runes[:limit-1]) + "…"
}
