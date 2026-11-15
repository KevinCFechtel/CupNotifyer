package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestHandlerFormatsReadableLogLine(t *testing.T) {
	var output bytes.Buffer
	handler := NewHandler(&output, slog.LevelInfo)
	record := slog.NewRecord(
		time.Date(2026, time.November, 15, 7, 32, 52, 0, time.UTC),
		slog.LevelError,
		"healthcheck failed",
		0,
	)
	record.Add(
		slog.Any("error", errors.New("connection refused")),
		slog.String("endpoint", "https://example.com"),
	)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	want := "2026-11-15 07:32:52 [ERROR] healthcheck failed: connection refused (endpoint=https://example.com)\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestHandlerKeepsEachRecordOnOneLine(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewHandler(&output, slog.LevelDebug))
	logger.Info("first\nsecond", "value", "one\ntwo")

	if strings.Count(output.String(), "\n") != 1 {
		t.Fatalf("output contains multiple lines: %q", output.String())
	}
	if !strings.Contains(output.String(), `first\nsecond`) {
		t.Fatalf("message newline was not escaped: %q", output.String())
	}
}
