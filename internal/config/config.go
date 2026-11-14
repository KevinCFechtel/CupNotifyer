package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultPollInterval = 5 * time.Minute
	defaultHTTPTimeout  = 15 * time.Second
	defaultStateFile    = "/data/state.json"
)

type Config struct {
	CupAPIURL        string
	PollInterval     time.Duration
	HTTPTimeout      time.Duration
	StateFile        string
	TelegramBotToken string
	TelegramChatID   string
	LogLevel         slog.Level
}

func Load() (Config, error) {
	cfg := Config{
		CupAPIURL:        strings.TrimSpace(os.Getenv("CUP_API_URL")),
		StateFile:        envOrDefault("STATE_FILE", defaultStateFile),
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramChatID:   strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")),
		LogLevel:         slog.LevelInfo,
	}

	var err error
	if cfg.PollInterval, err = durationFromEnv("POLL_INTERVAL", defaultPollInterval); err != nil {
		return Config{}, err
	}
	if cfg.HTTPTimeout, err = durationFromEnv("HTTP_TIMEOUT", defaultHTTPTimeout); err != nil {
		return Config{}, err
	}

	switch strings.ToLower(envOrDefault("LOG_LEVEL", "info")) {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "info":
		cfg.LogLevel = slog.LevelInfo
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		return Config{}, errors.New("LOG_LEVEL must be one of debug, info, warn, or error")
	}

	if cfg.CupAPIURL == "" {
		return Config{}, errors.New("CUP_API_URL is required")
	}
	parsedURL, err := url.ParseRequestURI(cfg.CupAPIURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return Config{}, errors.New("CUP_API_URL must be a valid http or https URL")
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.TelegramChatID == "" {
		return Config{}, errors.New("TELEGRAM_CHAT_ID is required")
	}
	if strings.TrimSpace(cfg.StateFile) == "" {
		return Config{}, errors.New("STATE_FILE must not be empty")
	}

	return cfg, nil
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return value, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
