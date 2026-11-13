// Package config defines AIOS' configuration model, safe defaults for weak
// hardware, YAML loading, environment overrides and validation.
package config

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from Go duration strings
// ("30s", "15m") in YAML and from environment variables.
type Duration time.Duration

func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	return d.UnmarshalText([]byte(s))
}

func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(strings.TrimSpace(string(text)))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// Config is the root configuration document.
type Config struct {
	App           AppConfig           `yaml:"app"`
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Cache         CacheConfig         `yaml:"cache"`
	Queue         QueueConfig         `yaml:"queue"`
	Vector        VectorConfig        `yaml:"vector"`
	Security      SecurityConfig      `yaml:"security"`
	LLM           LLMConfig           `yaml:"llm"`
	Voice         VoiceConfig         `yaml:"voice"`
	MQTT          MQTTConfig          `yaml:"mqtt"`
	HomeAssistant HomeAssistantConfig `yaml:"home_assistant"`
	Plugins       PluginConfig        `yaml:"plugins"`
	Memory        MemoryConfig        `yaml:"memory"`
	Observability ObservabilityConfig `yaml:"observability"`
	Scheduler     SchedulerConfig     `yaml:"scheduler"`
	Tools         ToolsConfig         `yaml:"tools"`
}

type AppConfig struct {
	Name       string `yaml:"name"`
	Env        string `yaml:"env"` // development | production
	DataDir    string `yaml:"data_dir"`
	InstanceID string `yaml:"instance_id"`
	LogLevel   string `yaml:"log_level"`
	LogFormat  string `yaml:"log_format"` // json | text
}

type ServerConfig struct {
	Addr            string          `yaml:"addr"`
	PublicURL       string          `yaml:"public_url"`
	ReadTimeout     Duration        `yaml:"read_timeout"`
	WriteTimeout    Duration        `yaml:"write_timeout"`
	IdleTimeout     Duration        `yaml:"idle_timeout"`
	ShutdownTimeout Duration        `yaml:"shutdown_timeout"`
	MaxBodyBytes    int64           `yaml:"max_body_bytes"`
	CORSOrigins     []string        `yaml:"cors_origins"`
	TrustedProxies  []string        `yaml:"trusted_proxies"`
	EnableGraphQL   bool            `yaml:"enable_graphql"`
	EnablePprof     bool            `yaml:"enable_pprof"`
	RateLimit       RateLimitConfig `yaml:"rate_limit"`
	TLS             TLSConfig       `yaml:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	Burst             int  `yaml:"burst"`
	AuthPerMinute     int  `yaml:"auth_per_minute"`
}

type DatabaseConfig struct {
	Driver          string   `yaml:"driver"` // sqlite | postgres
	DSN             string   `yaml:"dsn"`
	MaxOpenConns    int      `yaml:"max_open_conns"`
	MaxIdleConns    int      `yaml:"max_idle_conns"`
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
	AutoMigrate     bool     `yaml:"auto_migrate"`
	Seed            bool     `yaml:"seed"`
}

type CacheConfig struct {
	Driver     string      `yaml:"driver"` // memory | redis
	MaxEntries int         `yaml:"max_entries"`
	DefaultTTL Duration    `yaml:"default_ttl"`
	Prefix     string      `yaml:"prefix"`
	Redis      RedisConfig `yaml:"redis"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type QueueConfig struct {
	Driver            string   `yaml:"driver"` // memory | redis
	Workers           int      `yaml:"workers"`
	MaxAttempts       int      `yaml:"max_attempts"`
	VisibilityTimeout Duration `yaml:"visibility_timeout"`
	PollInterval      Duration `yaml:"poll_interval"`
}

type VectorConfig struct {
	Driver       string   `yaml:"driver"` // local | qdrant
	QdrantURL    string   `yaml:"qdrant_url"`
	QdrantAPIKey string   `yaml:"qdrant_api_key"`
	Collection   string   `yaml:"collection"`
	Dim          int      `yaml:"dim"`
	Timeout      Duration `yaml:"timeout"`
}
