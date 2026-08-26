// Package config loads process configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	RedisAddr       string

	// SMTP* configure outgoing mail for the verify-email/forgot-password
	// OTP emails (internal/mailer) - a real authenticated relay (Gmail,
	// SES, Brevo, Resend, etc), required at startup same as
	// DATABASE_URL/JWT_SECRET. See .env.example for Gmail setup.
	SMTPHost     string
	SMTPPort     string
	SMTPFrom     string
	SMTPUser     string
	SMTPPassword string
}

func Load() (Config, error) {
	cfg := Config{
		Port:            getenv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		RedisAddr:       getenv("REDIS_ADDR", "localhost:6379"),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        getenv("SMTP_PORT", "587"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return cfg, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}
	if cfg.SMTPHost == "" || cfg.SMTPFrom == "" || cfg.SMTPUser == "" || cfg.SMTPPassword == "" {
		return cfg, fmt.Errorf("SMTP_HOST, SMTP_FROM, SMTP_USER, and SMTP_PASSWORD are required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
