package notification

import (
	"context"
	"time"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

type Startup struct {
	PollInterval time.Duration
}

type Notifier interface {
	ID() string
	SendStartup(context.Context, Startup) error
	Send(context.Context, update.Available) error
}
