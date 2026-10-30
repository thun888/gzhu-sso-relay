package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    Server   `yaml:"server"`
	SecretKey string   `yaml:"secret_key"`
	Database  Database `yaml:"database"`
	Redis     Redis    `yaml:"redis"`
	Upstream  Upstream `yaml:"upstream"`
}

type Server struct {
	Addr           string `yaml:"addr"`
	BaseURL        string `yaml:"base_url"`
	LoginURL       string `yaml:"login_url"`
	EmailDomain    string `yaml:"email_domain"`
	SigningKeyPath string `yaml:"signing_key_path"`
}

type Database struct {
	DSN string `yaml:"dsn"`
}

type Redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type Upstream struct {
	CASBaseURL    string   `yaml:"cas_base_url"`
	PortalBaseURL string   `yaml:"portal_base_url"`
	Timeout       Duration `yaml:"timeout"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("配置: 读取 %s: %w", path, err)
	}

	cfg := defaults()
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("配置: 解析 %s: %w", path, err)
	}
	cfg.normalize()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("配置: %s: %w", path, err)
	}
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: Server{
			Addr:           ":8080",
			BaseURL:        "http://localhost:8080",
			LoginURL:       "/login",
			EmailDomain:    "gzhu.edu.cn",
			SigningKeyPath: "./data/signing_key.pem",
		},
		Redis: Redis{Addr: "localhost:6379"},
		Upstream: Upstream{
			CASBaseURL:    "https://newcas.gzhu.edu.cn",
			PortalBaseURL: "https://newmy.gzhu.edu.cn",
			Timeout:       Duration(30 * time.Second),
		},
	}
}

func (c *Config) normalize() {
	c.Server.BaseURL = strings.TrimSuffix(c.Server.BaseURL, "/")
	if c.Server.LoginURL == "" {
		c.Server.LoginURL = "/login"
	}
}

func (c *Config) validate() error {
	var missing []string
	if c.Database.DSN == "" {
		missing = append(missing, "database.dsn")
	}
	if c.SecretKey == "" {
		missing = append(missing, "secret_key")
	}
	if c.Server.BaseURL == "" {
		missing = append(missing, "server.base_url")
	}
	if len(missing) > 0 {
		return errors.New("缺少必需项: " + strings.Join(missing, ", "))
	}

	if c.Upstream.Timeout <= 0 {
		return errors.New("upstream.timeout 必须为正数")
	}
	return nil
}

// Duration 让配置文件可以直接写 30s、1m 这类时长。
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("时长需要是字符串: %w", err)
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("时长 %q 不合法，示例 30s: %w", raw, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
}
