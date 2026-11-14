package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

func TestSend(t *testing.T) {
	var received sendMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsecret/sendMessage" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	notifier := New("secret", "-1001", server.Client())
	notifier.apiBaseURL = server.URL
	err := notifier.Send(context.Background(), update.Available{
		Reference:      "ghcr.io/example/app:v1",
		Server:         "home",
		Kind:           update.KindVersion,
		UpdateType:     "major",
		CurrentVersion: "1.0.0",
		NewVersion:     "2.0.0",
		NewTag:         "v2.0.0",
		URL:            "https://example.com/app",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if received.ChatID != "-1001" || received.ParseMode != "HTML" {
		t.Errorf("request = %#v", received)
	}
	if !strings.Contains(received.Text, "v2.0.0") || !strings.Contains(received.Text, "home") {
		t.Errorf("message = %q", received.Text)
	}
}

func TestFormatMessageEscapesHTMLAndRejectsUnsafeURL(t *testing.T) {
	message := formatMessage(update.Available{
		Reference:    "example/<app>:latest",
		Kind:         update.KindDigest,
		URL:          "javascript:alert(1)",
		RemoteDigest: "sha256:1234567890abcdef",
	})
	if !strings.Contains(message, "example/&lt;app&gt;:latest") {
		t.Errorf("message does not escape HTML: %q", message)
	}
	if strings.Contains(message, "javascript") {
		t.Errorf("message contains unsafe URL: %q", message)
	}
	if !strings.Contains(message, "sha256:1234567890ab") {
		t.Errorf("message does not shorten digest as expected: %q", message)
	}
}
