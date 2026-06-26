package utils

import (
	"os"
	"strings"
)

const (
	// DisableRateLimitingEnvVar toggles rate limiting middleware when set to a truthy value.
	DisableRateLimitingEnvVar    = "PFINDER_DISABLE_RATE_LIMITING"
	ApplicationEnvironmentEnvVar = "APP_ENV"
)

func EnvIsTruthy(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
