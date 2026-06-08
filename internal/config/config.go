package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
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

func getEnvSlice(key string) []string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return nil
	}
	var result []string
	for _, k := range splitComma(value) {
		k = trimSpace(k)
		if k != "" {
			result = append(result, k)
		}
	}
	return result
}

func splitComma(s string) []string {
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
