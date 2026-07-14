package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                string
	DatabaseURL             string
	RedisAddr               string
	RedisPassword           string
	RedisDB                 int
	JWTAccessSecret         string
	JWTRefreshSecret        string
	JWTIssuer               string
	JWTAccessTTL            time.Duration
	JWTRefreshTTL           time.Duration
	GoogleOAuthClientID     string
	AppBaseURL              string
	SMTPHost                string
	SMTPPort                string
	SMTPUsername            string
	SMTPPassword            string
	SMTPFromEmail           string
	IdentifierRotationEvery time.Duration
	IdentifierRotationTick  time.Duration
	FirebaseCredentialsFile string
	MediaStorageDriver      string
	MediaStorageLocalRoot   string
	MediaS3Bucket           string
	MediaS3Region           string
	MediaS3Prefix           string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:                getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		RedisAddr:               getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:           os.Getenv("REDIS_PASSWORD"),
		RedisDB:                 mustInt("REDIS_DB", 0),
		JWTAccessSecret:         os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret:        os.Getenv("JWT_REFRESH_SECRET"),
		JWTIssuer:               getEnv("JWT_ISSUER", "xend-server"),
		JWTAccessTTL:            mustDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:           mustDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		GoogleOAuthClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		AppBaseURL:              getEnv("APP_BASE_URL", "http://localhost:8080"),
		SMTPHost:                os.Getenv("SMTP_HOST"),
		SMTPPort:                getEnv("SMTP_PORT", "587"),
		SMTPUsername:            os.Getenv("SMTP_USERNAME"),
		SMTPPassword:            os.Getenv("SMTP_PASSWORD"),
		SMTPFromEmail:           os.Getenv("SMTP_FROM_EMAIL"),
		IdentifierRotationEvery: mustDuration("IDENTIFIER_ROTATION_EVERY", 0),
		IdentifierRotationTick:  mustDuration("IDENTIFIER_ROTATION_TICK", 2*time.Minute),
		FirebaseCredentialsFile: os.Getenv("FIREBASE_CREDENTIALS_FILE"),
		MediaStorageDriver:      getEnv("MEDIA_STORAGE_DRIVER", "local"),
		MediaStorageLocalRoot:   getEnv("MEDIA_STORAGE_LOCAL_ROOT", "storage"),
		MediaS3Bucket:           os.Getenv("MEDIA_S3_BUCKET"),
		MediaS3Region:           getEnv("MEDIA_S3_REGION", os.Getenv("AWS_REGION")),
		MediaS3Prefix:           os.Getenv("MEDIA_S3_PREFIX"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTAccessSecret == "" || cfg.JWTRefreshSecret == "" {
		return Config{}, fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
	}
	cfg.MediaStorageDriver = strings.ToLower(strings.TrimSpace(cfg.MediaStorageDriver))
	if cfg.MediaStorageDriver == "s3" && strings.TrimSpace(cfg.MediaS3Bucket) == "" {
		return Config{}, fmt.Errorf("MEDIA_S3_BUCKET is required when MEDIA_STORAGE_DRIVER=s3")
	}

	return cfg, nil
}

func mustDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err == nil {
		return d
	}
	if hours, convErr := strconv.Atoi(raw); convErr == nil {
		return time.Duration(hours) * time.Hour
	}
	return fallback
}

func mustInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
