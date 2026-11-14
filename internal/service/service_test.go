package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/KevinCFechtel/CupNotifyer/internal/notification"
	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

type fakeSource struct {
	updates []update.Available
	err     error
}

func (f *fakeSource) FetchUpdates(context.Context) ([]update.Available, error) {
	return f.updates, f.err
}

type memoryStore struct {
	sent map[string]string
}

func (m *memoryStore) IsSent(targetID, imageID, updateID string) bool {
	return m.sent[targetID+"|"+imageID] == updateID
}

func (m *memoryStore) MarkSent(targetID, imageID, updateID string) error {
	m.sent[targetID+"|"+imageID] = updateID
	return nil
}

type fakeNotifier struct {
	id           string
	sends        int
	startupSends int
	sendErr      error
	startupErr   error
}

func (f *fakeNotifier) ID() string {
	return f.id
}

func (f *fakeNotifier) Send(context.Context, update.Available) error {
	f.sends++
	return f.sendErr
}

func (f *fakeNotifier) SendStartup(context.Context, notification.Startup) error {
	f.startupSends++
	return f.startupErr
}

func TestSendStartupNotifiesEveryTarget(t *testing.T) {
	first := &fakeNotifier{id: "pushover:1"}
	second := &fakeNotifier{id: "webhook:1", startupErr: errors.New("temporary failure")}
	service := New(
		&fakeSource{},
		&memoryStore{sent: make(map[string]string)},
		[]notification.Notifier{first, second},
		5*time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	service.sendStartup(context.Background())

	if first.startupSends != 1 || second.startupSends != 1 {
		t.Fatalf("startup send counts = %d, %d; want 1, 1", first.startupSends, second.startupSends)
	}
}

func TestPollDeduplicatesPerTarget(t *testing.T) {
	source := &fakeSource{updates: []update.Available{{
		Reference: "example/app:v1",
		Kind:      update.KindVersion,
		NewTag:    "v2",
	}}}
	store := &memoryStore{sent: make(map[string]string)}
	first := &fakeNotifier{id: "pushover:1"}
	second := &fakeNotifier{id: "webhook:1"}
	service := New(
		source,
		store,
		[]notification.Notifier{first, second},
		time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	service.poll(context.Background())
	service.poll(context.Background())

	if first.sends != 1 || second.sends != 1 {
		t.Fatalf("send counts = %d, %d; want 1, 1", first.sends, second.sends)
	}

	source.updates[0].NewTag = "v3"
	service.poll(context.Background())
	if first.sends != 2 || second.sends != 2 {
		t.Fatalf("send counts after new update = %d, %d; want 2, 2", first.sends, second.sends)
	}
}

func TestPollRetriesFailedTarget(t *testing.T) {
	source := &fakeSource{updates: []update.Available{{
		Reference:    "example/app:latest",
		Kind:         update.KindDigest,
		RemoteDigest: "sha256:new",
	}}}
	store := &memoryStore{sent: make(map[string]string)}
	failing := &fakeNotifier{id: "pushover:1", sendErr: errors.New("temporary failure")}
	successful := &fakeNotifier{id: "webhook:1"}
	service := New(
		source,
		store,
		[]notification.Notifier{failing, successful},
		time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	service.poll(context.Background())
	failing.sendErr = nil
	service.poll(context.Background())

	if failing.sends != 2 {
		t.Errorf("failed target sends = %d, want 2", failing.sends)
	}
	if successful.sends != 1 {
		t.Errorf("successful target sends = %d, want 1", successful.sends)
	}
}
