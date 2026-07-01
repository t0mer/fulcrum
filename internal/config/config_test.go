package config

import (
	"testing"

	"github.com/spf13/pflag"
)

func loadWith(t *testing.T, args []string, env map[string]string) (*Config, error) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	BindFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return Load(fs)
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := loadWith(t, nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Match.DefaultThreshold != 0.48 {
		t.Errorf("threshold = %v, want 0.48", cfg.Match.DefaultThreshold)
	}
	if cfg.Provider.Name != "gowa" {
		t.Errorf("provider = %q, want gowa", cfg.Provider.Name)
	}
}

func TestPrecedenceFlagOverEnv(t *testing.T) {
	// Flag beats env beats default.
	cfg, err := loadWith(t, []string{"--server.port=9999"}, map[string]string{
		"FULCRUM_SERVER_PORT": "7000",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("port = %d, want 9999 (flag wins over env)", cfg.Server.Port)
	}
}

func TestPrecedenceEnvOverDefault(t *testing.T) {
	cfg, err := loadWith(t, nil, map[string]string{
		"FULCRUM_PROVIDER_NAME": "greenapi",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider.Name != "greenapi" {
		t.Errorf("provider = %q, want greenapi (env wins over default)", cfg.Provider.Name)
	}
}

func TestValidateRejectsBadProvider(t *testing.T) {
	_, err := loadWith(t, nil, map[string]string{"FULCRUM_PROVIDER_NAME": "nope"})
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
}

func TestValidateRejectsBadThreshold(t *testing.T) {
	_, err := loadWith(t, nil, map[string]string{"FULCRUM_MATCH_DEFAULT_THRESHOLD": "1.5"})
	if err == nil {
		t.Fatal("expected error for out-of-range threshold")
	}
}
