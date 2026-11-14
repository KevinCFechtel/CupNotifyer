package notification

import (
	"context"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

type Notifier interface {
	ID() string
	Send(context.Context, update.Available) error
}
