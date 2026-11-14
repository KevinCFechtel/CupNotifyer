package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/KevinCFechtel/CupNotifyer/internal/config"
	"github.com/KevinCFechtel/CupNotifyer/internal/cup"
	"github.com/KevinCFechtel/CupNotifyer/internal/notification"
	"github.com/KevinCFechtel/CupNotifyer/internal/notification/pushover"
	"github.com/KevinCFechtel/CupNotifyer/internal/service"
	"github.com/KevinCFechtel/CupNotifyer/internal/state"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	cupClient := cup.NewClient(cfg.CupAPIURL, httpClient)
	store, err := state.NewFileStore(cfg.StateFile)
	if err != nil {
		logger.Error("failed to initialize state", "error", err)
		os.Exit(1)
	}

	notifiers := []notification.Notifier{
		pushover.New(cfg.PushoverAPIToken, cfg.PushoverUserKey, httpClient),
	}
	runner := service.New(cupClient, store, notifiers, cfg.PollInterval, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("cup notifier started",
		"cup_api_url", cfg.CupAPIURL,
		"poll_interval", cfg.PollInterval,
		"notification_targets", len(notifiers),
	)
	runner.Run(ctx)
	logger.Info("cup notifier stopped")
}
