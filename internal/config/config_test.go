package config

import (
	"testing"
	"time"
)

func valid() Config {
	return Config{
		Environment:          "development",
		HTTPPort:             "8085",
		AuthPublicKeyBase64:  "key",
		AuthIssuer:           "atm-auth-dev",
		AllowedSystems:       []string{"lavendimia"},
		MaxConcurrentRenders: 4,
		RenderTimeout:        15 * time.Second,
	}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := map[string]func(*Config){
		"bad environment": func(c *Config) { c.Environment = "staging" },
		"no public key":   func(c *Config) { c.AuthPublicKeyBase64 = "" },
		"no issuer":       func(c *Config) { c.AuthIssuer = "" },
		"no systems":      func(c *Config) { c.AllowedSystems = nil },
		"bad port":        func(c *Config) { c.HTTPPort = "99999" },
		"no renders":      func(c *Config) { c.MaxConcurrentRenders = 0 },
		"too many":        func(c *Config) { c.MaxConcurrentRenders = 100 },
		"tiny timeout":    func(c *Config) { c.RenderTimeout = time.Millisecond },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := valid()
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadParsesAllowedSystems(t *testing.T) {
	t.Setenv("AUTH_PUBLIC_KEY_BASE64", "key")
	t.Setenv("AUTH_ISSUER", "atm-auth-dev")
	t.Setenv("ALLOWED_SYSTEMS", " lavendimia, pulso ,,")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.AllowedSystems) != 2 || c.AllowedSystems[0] != "lavendimia" || c.AllowedSystems[1] != "pulso" {
		t.Fatalf("AllowedSystems = %v", c.AllowedSystems)
	}
}
