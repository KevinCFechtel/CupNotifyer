package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/KevinCFechtel/CupNotifyer/internal/notification"
	"github.com/KevinCFechtel/CupNotifyer/internal/state"
	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

type UpdateSource interface {
	FetchUpdates(context.Context) ([]update.Available, error)
}

type Service struct {
	source       UpdateSource
	store        state.Store
	notifiers    []notification.Notifier
	pollInterval time.Duration
	logger       *slog.Logger
}

func New(
	source UpdateSource,
	store state.Store,
	notifiers []notification.Notifier,
	pollInterval time.Duration,
	logger *slog.Logger,
) *Service {
	return &Service{
		source:       source,
		store:        store,
		notifiers:    notifiers,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

func (s *Service) Run(ctx context.Context) {
	s.poll(ctx)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

func (s *Service) poll(ctx context.Context) {
	updates, err := s.source.FetchUpdates(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error("failed to fetch Cup updates", "error", err)
		}
		return
	}

	s.logger.Info("Cup poll completed", "updates_available", len(updates))
	for _, available := range updates {
		for _, notifier := range s.notifiers {
			if s.store.IsSent(notifier.ID(), available.ImageID(), available.UpdateID()) {
				continue
			}
			if err := notifier.Send(ctx, available); err != nil {
				if ctx.Err() == nil {
					s.logger.Error("failed to send notification",
						"target", notifier.ID(),
						"image", available.Reference,
						"update_id", available.UpdateID(),
						"error", err,
					)
				}
				continue
			}
			if err := s.store.MarkSent(notifier.ID(), available.ImageID(), available.UpdateID()); err != nil {
				s.logger.Error("notification sent but state could not be persisted",
					"target", notifier.ID(),
					"image", available.Reference,
					"update_id", available.UpdateID(),
					"error", err,
				)
				continue
			}
			s.logger.Info("notification sent",
				"target", notifier.ID(),
				"image", available.Reference,
				"update_id", available.UpdateID(),
			)
		}
	}
}
