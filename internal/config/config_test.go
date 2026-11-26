package config

import "testing"

func TestLoadPushoverEncryptionKey(t *testing.T) {
	t.Setenv("CUP_API_URL", "http://cup:8000/api/v3/json")
	t.Setenv("PUSHOVER_API_TOKEN", "token")
	t.Setenv("PUSHOVER_USER_KEY", "user")
	t.Setenv("POLL_INTERVAL", "")
	t.Setenv("HTTP_TIMEOUT", "")
	t.Setenv("STATE_FILE", "")
	t.Setenv("LOG_LEVEL", "")
	for _, key := range []string{"", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", " "} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("PUSHOVER_ENCRYPTION_KEY", key)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			// Validation happens in the notifier constructor. In particular,
			// do not turn whitespace-only invalid keys into disabled E2EE.
			if cfg.PushoverEncryptionKey != key {
				t.Fatal("configured encryption key was not preserved")
			}
		})
	}
}
