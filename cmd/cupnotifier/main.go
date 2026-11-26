package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/KevinCFechtel/CupNotifyer/internal/config"
	"github.com/KevinCFechtel/CupNotifyer/internal/cup"
	"github.com/KevinCFechtel/CupNotifyer/internal/health"
	"github.com/KevinCFechtel/CupNotifyer/internal/logging"
	"github.com/KevinCFechtel/CupNotifyer/internal/notification"
	"github.com/KevinCFechtel/CupNotifyer/internal/notification/pushover"
	"github.com/KevinCFechtel/CupNotifyer/internal/service"
	"github.com/KevinCFechtel/CupNotifyer/internal/state"
)

func main() {
	slog.SetDefault(slog.New(logging.NewHandler(os.Stdout, slog.LevelInfo)))

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		runHealthcheck()
		return
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(logging.NewHandler(os.Stdout, cfg.LogLevel))
	slog.SetDefault(logger)

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	pushoverNotifier, err := pushover.New(cfg.PushoverAPIToken, cfg.PushoverUserKey, cfg.PushoverEncryptionKey, httpClient)
	if err != nil {
		logger.Error("failed to initialize Pushover", "error", err)
		os.Exit(1)
	}
	cupClient := cup.NewClient(cfg.CupAPIURL, httpClient)
	store, err := state.NewFileStore(cfg.StateFile)
	if err != nil {
		logger.Error("failed to initialize state", "error", err)
		os.Exit(1)
	}

	notifiers := []notification.Notifier{
		pushoverNotifier,
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

func runHealthcheck() {
	endpoint := strings.TrimSpace(os.Getenv("HEALTHCHECK_URL"))
	if endpoint == "" {
		endpoint = health.DefaultEndpoint
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	checker := health.NewChecker(endpoint, &http.Client{})
	if err := checker.Check(ctx); err != nil {
		slog.Error("healthcheck failed", "error", err)
		os.Exit(1)
	}
}
