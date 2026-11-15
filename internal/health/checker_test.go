package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckSucceedsWhenExternalEndpointResponds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "CupNotifyer-Healthcheck/1" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	checker := NewChecker(server.URL, server.Client())
	if err := checker.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestCheckFailsWithoutConnectivity(t *testing.T) {
	checker := NewChecker("http://127.0.0.1:1", http.DefaultClient)
	if err := checker.Check(context.Background()); err == nil {
		t.Fatal("Check() error = nil, want an error")
	}
}
