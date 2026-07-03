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
	if cfg.Provider.Name != "greenapi" {
		t.Errorf("provider = %q, want greenapi", cfg.Provider.Name)
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

func TestEnvBindsSecretAndProviderCreds(t *testing.T) {
	// These keys have no meaningful default; they must still bind from env
	// (regression guard for the Viper AutomaticEnv + Unmarshal gotcha).
	cfg, err := loadWith(t, nil, map[string]string{
		"FULCRUM_SERVER_WEBHOOK_SECRET":     "s3cr3t",
		"FULCRUM_PROVIDER_BASE_URL":         "http://gw:3000",
		"FULCRUM_PROVIDER_TOKEN":            "user:pass",
		"FULCRUM_SINK_DESTINATION_GROUP_ID": "123@g.us",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.WebhookSecret != "s3cr3t" {
		t.Errorf("webhook_secret = %q, want s3cr3t", cfg.Server.WebhookSecret)
	}
	if cfg.Provider.BaseURL != "http://gw:3000" || cfg.Provider.Token != "user:pass" {
		t.Errorf("provider creds not bound: %+v", cfg.Provider)
	}
	if cfg.Sink.DestinationGroupID != "123@g.us" {
		t.Errorf("destination_group_id = %q", cfg.Sink.DestinationGroupID)
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
