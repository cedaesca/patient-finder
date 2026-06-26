package config

const (
	JWTAccessTokenSecretKey  = "JWT_ACCESS_TOKEN_SECRET"
	JWTRefreshTokenSecretKey = "JWT_REFRESH_TOKEN_SECRET"
	DatabaseURLKey           = "DATABASE_URL"
	EnvironmentKey           = "APP_ENV"
	NatsStreamName           = "NATS_STREAM_NAME"
	NatsDurablePrefix        = "NATS_DURABLE_PREFIX"
	WebClientURL             = "WEB_CLIENT_URL"
	CORSAllowedOriginsKey    = "CORS_ALLOWED_ORIGINS"

	// Logging
	LogLevelKey  = "LOG_LEVEL"
	LogFormatKey = "LOG_FORMAT"
)
