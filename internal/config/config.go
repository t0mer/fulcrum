// Package config loads Fulcrum configuration with Viper.
//
// Precedence is flags > env > YAML file > built-in defaults. Environment
// variables use the FULCRUM_ prefix with "." replaced by "_"
// (e.g. server.port -> FULCRUM_SERVER_PORT).
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	ML     MLConfig     `mapstructure:"ml"`
	Match  MatchConfig  `mapstructure:"match"`
	Enroll EnrollConfig `mapstructure:"enroll"`
	Queue  QueueConfig  `mapstructure:"queue"`
	Sink   SinkConfig   `mapstructure:"sink"`
	DBPath string       `mapstructure:"db_path"`

	Provider ProviderConfig `mapstructure:"provider"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port          int    `mapstructure:"port"`
	WebhookSecret string `mapstructure:"webhook_secret"`
	LogLevel      string `mapstructure:"log_level"`
}

// ProviderConfig selects and configures the active WhatsApp gateway adapter.
type ProviderConfig struct {
	Name    string `mapstructure:"name"` // gowa | greenapi | wwebjs
	BaseURL string `mapstructure:"base_url"`
	Token   string `mapstructure:"token"`
}

// MLConfig points at the fulcrum-ml sidecar.
type MLConfig struct {
	URL      string  `mapstructure:"url"`
	DetScore float64 `mapstructure:"det_score"`
}

// MatchConfig holds cosine-matching thresholds.
type MatchConfig struct {
	DefaultThreshold float64 `mapstructure:"default_threshold"`
}

// EnrollConfig holds enrollment (reference photo) settings.
type EnrollConfig struct {
	FacesPath string `mapstructure:"faces_path"`
}

// QueueConfig holds durable job-queue / worker-pool settings.
type QueueConfig struct {
	Workers     int `mapstructure:"workers"`
	MaxAttempts int `mapstructure:"max_attempts"`
}

// SinkConfig holds match-output sink settings.
type SinkConfig struct {
	Mode               string `mapstructure:"mode"` // storage-only | forward-only | both
	StoragePath        string `mapstructure:"storage_path"`
	DestinationGroupID string `mapstructure:"destination_group_id"`
}

// BindFlags registers the CLI flags that override config. It does not parse;
// the caller parses the flag set and passes it to Load.
func BindFlags(fs *pflag.FlagSet) {
	fs.String("config", "", "path to YAML config file")
	fs.Int("server.port", 8080, "HTTP listen port")
	fs.String("server.log_level", "info", "log level: debug|info|warning|error")
	fs.String("provider.name", "gowa", "WhatsApp provider: gowa|greenapi|wwebjs")
	fs.String("ml.url", "http://fulcrum-ml:8081", "fulcrum-ml sidecar base URL")
	fs.Int("queue.workers", 2, "number of queue workers")
}

// Load resolves configuration from flags, env, and an optional YAML file.
// The flag set must already be parsed.
func Load(fs *pflag.FlagSet) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetEnvPrefix("FULCRUM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.BindPFlags(fs); err != nil {
		return nil, fmt.Errorf("binding flags: %w", err)
	}

	if path := v.GetString("config"); path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config %q: %w", path, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.log_level", "info")
	// Every key needs a default (even empty) so Viper's AutomaticEnv binds it
	// through Unmarshal — otherwise env vars for keys without a default (e.g.
	// FULCRUM_SERVER_WEBHOOK_SECRET, FULCRUM_PROVIDER_TOKEN) are silently ignored.
	v.SetDefault("server.webhook_secret", "")
	v.SetDefault("db_path", "/data/fulcrum.db")
	v.SetDefault("provider.name", "gowa")
	v.SetDefault("provider.base_url", "")
	v.SetDefault("provider.token", "")
	v.SetDefault("ml.url", "http://fulcrum-ml:8081")
	v.SetDefault("ml.det_score", 0.5)
	v.SetDefault("match.default_threshold", 0.48)
	v.SetDefault("enroll.faces_path", "/data/faces")
	v.SetDefault("queue.workers", 2)
	v.SetDefault("queue.max_attempts", 5)
	v.SetDefault("sink.mode", "both")
	v.SetDefault("sink.storage_path", "/data/matches")
	v.SetDefault("sink.destination_group_id", "")
}

func (c *Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range", c.Server.Port)
	}
	switch c.Provider.Name {
	case "gowa", "greenapi", "wwebjs":
	default:
		return fmt.Errorf("provider.name %q must be gowa|greenapi|wwebjs", c.Provider.Name)
	}
	switch c.Sink.Mode {
	case "storage-only", "forward-only", "both":
	default:
		return fmt.Errorf("sink.mode %q must be storage-only|forward-only|both", c.Sink.Mode)
	}
	if c.Queue.Workers < 1 {
		return fmt.Errorf("queue.workers must be >= 1")
	}
	if c.Match.DefaultThreshold <= 0 || c.Match.DefaultThreshold >= 1 {
		return fmt.Errorf("match.default_threshold %.2f must be in (0,1)", c.Match.DefaultThreshold)
	}
	return nil
}
