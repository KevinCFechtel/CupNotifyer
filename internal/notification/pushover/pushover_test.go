package pushover

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

func TestSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm(): %v", err)
		}
		if r.Form.Get("token") != "app-token" {
			t.Errorf("token = %q", r.Form.Get("token"))
		}
		if r.Form.Get("user") != "user-key" {
			t.Errorf("user = %q", r.Form.Get("user"))
		}
		if !strings.Contains(r.Form.Get("message"), "v2.0.0") {
			t.Errorf("message = %q", r.Form.Get("message"))
		}
		if r.Form.Get("url") != "https://example.com/app" {
			t.Errorf("url = %q", r.Form.Get("url"))
		}
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id"}`))
	}))
	defer server.Close()

	notifier := New("app-token", "user-key", server.Client())
	notifier.apiURL = server.URL
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
	if strings.Contains(notifier.ID(), "user-key") {
		t.Fatal("notifier ID exposes the Pushover user key")
	}
}

func TestSendReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["user identifier is invalid"]}`))
	}))
	defer server.Close()

	notifier := New("app-token", "user-key", server.Client())
	notifier.apiURL = server.URL
	err := notifier.Send(context.Background(), update.Available{
		Reference:    "example/app:latest",
		Kind:         update.KindDigest,
		RemoteDigest: "sha256:new",
	})
	if err == nil || !strings.Contains(err.Error(), "user identifier is invalid") {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestFormatMessageAndURLValidation(t *testing.T) {
	message := formatMessage(update.Available{
		Reference:    "example/app:latest",
		Kind:         update.KindDigest,
		LocalDigests: []string{"sha256:old"},
		RemoteDigest: "sha256:1234567890abcdef",
	})
	if !strings.Contains(message, "sha256:1234567890ab") {
		t.Errorf("message does not shorten digest: %q", message)
	}
	if safeProjectURL("javascript:alert(1)") != "" {
		t.Fatal("unsafe URL was accepted")
	}
}

func TestTruncateMessage(t *testing.T) {
	message := strings.Repeat("ä", maxMessageRunes+10)
	got := truncateMessage(message)
	if len([]rune(got)) != maxMessageRunes {
		t.Fatalf("message length = %d, want %d", len([]rune(got)), maxMessageRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatal("truncated message has no ellipsis")
	}
}
