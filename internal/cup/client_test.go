package cup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

func TestFetchUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"images": [
				{
					"reference": "ghcr.io/example/app:v1.2.3",
					"url": "https://example.com/app",
					"result": {
						"has_update": true,
						"info": {
							"type": "version",
							"version_update_type": "minor",
							"new_tag": "v1.3.0",
							"current_version": "1.2.3",
							"new_version": "1.3.0"
						}
					},
					"server": "home"
				},
				{
					"reference": "example/worker:latest",
					"result": {
						"has_update": true,
						"info": {
							"type": "digest",
							"local_digests": ["sha256:old"],
							"remote_digest": "sha256:new"
						}
					},
					"server": null
				},
				{
					"reference": "example/current:latest",
					"result": {"has_update": false, "info": null}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, &http.Client{Timeout: time.Second})
	got, err := client.FetchUpdates(context.Background())
	if err != nil {
		t.Fatalf("FetchUpdates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(updates) = %d, want 2", len(got))
	}

	if got[0].Kind != update.KindVersion || got[0].NewTag != "v1.3.0" {
		t.Errorf("version update = %#v", got[0])
	}
	if got[0].Server != "home" || got[0].URL != "https://example.com/app" {
		t.Errorf("version metadata = %#v", got[0])
	}
	if got[1].Kind != update.KindDigest || got[1].RemoteDigest != "sha256:new" {
		t.Errorf("digest update = %#v", got[1])
	}
}

func TestFetchUpdatesRejectsInvalidUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"images": [{
				"reference": "example/app:latest",
				"result": {"has_update": true, "info": null}
			}]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	if _, err := client.FetchUpdates(context.Background()); err == nil {
		t.Fatal("FetchUpdates() error = nil, want an error")
	}
}
