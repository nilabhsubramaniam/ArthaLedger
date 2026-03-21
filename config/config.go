package config

import (
	"log/slog"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	SMTP     SMTPConfig
	OpenAI   OpenAIConfig
}

// AppConfig holds application-level settings
type AppConfig struct {
	Port int
	Env  string
}

// DatabaseConfig holds PostgreSQL configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string
	Port     int
	Password string
}

// JWTConfig holds JWT token configuration
type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// SMTPConfig holds email configuration
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// OpenAIConfig holds OpenAI API configuration
type OpenAIConfig struct {
	APIKey string
}

// Load loads configuration from .env file
func Load() *Config {
	// Set config file name and path
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	// Set environment variable prefix
	viper.SetEnvPrefix("")
	viper.AutomaticEnv()

	// Load config from file
	if err := viper.ReadInConfig(); err != nil {
		slog.Warn("Error loading .env file, using environment variables only", "error", err)
	}

	// Parse and validate configuration
	cfg := &Config{
		App: AppConfig{
			Port: viper.GetInt("APP_PORT"),
			Env:  viper.GetString("APP_ENV"),
		},
		Database: DatabaseConfig{
			Host:     viper.GetString("DB_HOST"),
			Port:     viper.GetInt("DB_PORT"),
			Name:     viper.GetString("DB_NAME"),
			User:     viper.GetString("DB_USER"),
			Password: viper.GetString("DB_PASSWORD"),
			SSLMode:  viper.GetString("DB_SSLMODE"),
		},
		Redis: RedisConfig{
			Host:     viper.GetString("REDIS_HOST"),
			Port:     viper.GetInt("REDIS_PORT"),
			Password: viper.GetString("REDIS_PASSWORD"),
		},
		JWT: JWTConfig{
			Secret:     viper.GetString("JWT_SECRET"),
			AccessTTL:  parseDuration(viper.GetString("JWT_ACCESS_TTL"), 15*time.Minute),
			RefreshTTL: parseDuration(viper.GetString("JWT_REFRESH_TTL"), 7*24*time.Hour),
		},
		SMTP: SMTPConfig{
			Host:     viper.GetString("SMTP_HOST"),
			Port:     viper.GetInt("SMTP_PORT"),
			User:     viper.GetString("SMTP_USER"),
			Password: viper.GetString("SMTP_PASSWORD"),
			From:     viper.GetString("SMTP_FROM"),
		},
		OpenAI: OpenAIConfig{
			APIKey: viper.GetString("OPENAI_API_KEY"),
		},
	}

	// Validate required settings
	if cfg.App.Port == 0 {
		cfg.App.Port = 8080
	}

	if cfg.JWT.Secret == "" {
		slog.Error("JWT_SECRET is not set. Please configure it in .env file")
		panic("JWT_SECRET is required")
	}

	if len(cfg.JWT.Secret) < 32 {
		slog.Error("JWT_SECRET must be at least 32 characters long")
		panic("JWT_SECRET too short")
	}

	slog.Info("Configuration loaded successfully",
		"port", cfg.App.Port,
		"env", cfg.App.Env,
		"db_host", cfg.Database.Host,
		"redis_host", cfg.Redis.Host,
	)

	return cfg
}

// parseDuration parses a duration string or returns a default
func parseDuration(durationStr string, defaultDuration time.Duration) time.Duration {
	if durationStr == "" {
		return defaultDuration
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		slog.Warn("Invalid duration format, using default", "value", durationStr, "error", err)
		return defaultDuration
	}

	return duration
}
