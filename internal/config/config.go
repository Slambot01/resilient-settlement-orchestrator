package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	PubSub   PubSubConfig
	Tracing  TracingConfig
	Log      LogConfig
	Auth     AuthConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int
	Env  string
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int
	MinConns int
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string
	Format string
}

// AuthConfig holds API key authentication settings.
type AuthConfig struct {
	APIKeys []string
}

// PubSubConfig holds Google Cloud Pub/Sub settings.
type PubSubConfig struct {
	ProjectID      string
	WebhookTopicID string
	EventTopicID   string
	SubscriptionID string
	DLQTopicID     string
	DLQSubID       string
	MaxRetries     int
	Enabled        bool
}

// TracingConfig holds OpenTelemetry distributed tracing settings.
type TracingConfig struct {
	Enabled    bool
	Endpoint   string  // OTLP HTTP endpoint (e.g., "http://jaeger:4318")
	SampleRate float64 // 0.0–1.0, fraction of traces to sample
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// Load reads configuration from environment variables with sensible defaults.
// In production, it validates that critical secrets are set and panics if not.
func Load() *Config {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvInt("SERVER_PORT", 8080),
			Env:  getEnv("SERVER_ENV", "development"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "payment_user"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "payment_orchestrator"),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
			MaxConns: getEnvInt("DB_MAX_CONNS", 25),
			MinConns: getEnvInt("DB_MIN_CONNS", 5),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "debug"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Auth: AuthConfig{
			APIKeys: getEnvSlice("API_KEYS"),
		},
		PubSub: PubSubConfig{
			ProjectID:      getEnv("PUBSUB_PROJECT_ID", "local-dev"),
			WebhookTopicID: getEnv("PUBSUB_WEBHOOK_TOPIC", "webhook-events"),
			EventTopicID:   getEnv("PUBSUB_EVENT_TOPIC", "payment-state-changes"),
			SubscriptionID: getEnv("PUBSUB_SUBSCRIPTION", "webhook-processor"),
			DLQTopicID:     getEnv("PUBSUB_DLQ_TOPIC", "webhook-events-dlq"),
			DLQSubID:       getEnv("PUBSUB_DLQ_SUBSCRIPTION", "webhook-dlq-reader"),
			MaxRetries:     getEnvInt("PUBSUB_MAX_RETRIES", 5),
			Enabled:        getEnv("PUBSUB_ENABLED", "false") == "true",
		},
		Tracing: TracingConfig{
			Enabled:    getEnv("TRACING_ENABLED", "false") == "true",
			Endpoint:   getEnv("TRACING_ENDPOINT", "http://localhost:4318"),
			SampleRate: getEnvFloat("TRACING_SAMPLE_RATE", 1.0),
		},
	}

	// Fail fast in production if critical secrets are missing
	if cfg.Server.Env == "production" {
		var missing []string
		if cfg.Database.Password == "" {
			missing = append(missing, "DB_PASSWORD")
		}
		if len(cfg.Auth.APIKeys) == 0 {
			missing = append(missing, "API_KEYS")
		}
		if len(missing) > 0 {
			panic(fmt.Sprintf("FATAL: required environment variables not set for production: %v", missing))
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvSlice(key string) []string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return nil
	}
	var result []string
	for _, k := range strings.Split(value, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			result = append(result, k)
		}
	}
	return result
}
