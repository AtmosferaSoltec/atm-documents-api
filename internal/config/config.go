package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the service configuration, read once from the environment.
type Config struct {
	Environment         string
	HTTPPort            string
	AuthPublicKeyBase64 string
	AuthIssuer          string
	// AllowedSystems are the auth system codes whose service token may call this service.
	AllowedSystems       []string
	MaxConcurrentRenders int
	RenderTimeout        time.Duration
}

// Load reads and validates the configuration.
func Load() (Config, error) {
	c := Config{
		Environment:          value("APP_ENV", "development"),
		HTTPPort:             value("HTTP_PORT", "8085"),
		AuthPublicKeyBase64:  os.Getenv("AUTH_PUBLIC_KEY_BASE64"),
		AuthIssuer:           os.Getenv("AUTH_ISSUER"),
		MaxConcurrentRenders: 4,
		RenderTimeout:        15 * time.Second,
	}
	for _, system := range strings.Split(value("ALLOWED_SYSTEMS", "lavendimia"), ",") {
		if system = strings.TrimSpace(system); system != "" {
			c.AllowedSystems = append(c.AllowedSystems, system)
		}
	}
	if raw := os.Getenv("MAX_CONCURRENT_RENDERS"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, errors.New("invalid MAX_CONCURRENT_RENDERS")
		}
		c.MaxConcurrentRenders = v
	}
	if raw := os.Getenv("RENDER_TIMEOUT"); raw != "" {
		v, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, errors.New("invalid RENDER_TIMEOUT")
		}
		c.RenderTimeout = v
	}
	return c, c.Validate()
}

// Validate rejects configurations that cannot run safely.
func (c Config) Validate() error {
	if c.Environment != "development" && c.Environment != "production" {
		return errors.New("APP_ENV must be development or production")
	}
	for name, v := range map[string]string{
		"AUTH_PUBLIC_KEY_BASE64": c.AuthPublicKeyBase64,
		"AUTH_ISSUER":            c.AuthIssuer,
	} {
		if v == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(c.AllowedSystems) == 0 {
		return errors.New("ALLOWED_SYSTEMS must list at least one system")
	}
	port, err := strconv.Atoi(c.HTTPPort)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("invalid HTTP_PORT")
	}
	if c.MaxConcurrentRenders < 1 || c.MaxConcurrentRenders > 32 {
		return errors.New("MAX_CONCURRENT_RENDERS must be between 1 and 32")
	}
	if c.RenderTimeout < time.Second || c.RenderTimeout > 2*time.Minute {
		return errors.New("RENDER_TIMEOUT must be between 1s and 2m")
	}
	return nil
}

func value(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
