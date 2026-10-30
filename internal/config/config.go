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
	Addr           string       `yaml:"addr"`
	BaseURL        string       `yaml:"base_url"`
	LoginURL       string       `yaml:"login_url"`
	SiteTitle      string       `yaml:"site_title"`
	EmailDomains   EmailDomains `yaml:"email_domains"`
	SigningKeyPath string       `yaml:"signing_key_path"`
	LoginPanel     LoginPanel   `yaml:"login_panel"`
}

// LoginPanel 控制登录页右半区的预留文案，enabled 为 false 时登录卡片收成单栏。
type LoginPanel struct {
	Enabled bool   `yaml:"enabled"`
	Title   string `yaml:"title"`
	Body    string `yaml:"body"`
}

// EmailDomains 按 sub 前缀匹配邮箱后缀，取最长匹配。键为空字符串时匹配所有 sub，可作兜底。
type EmailDomains map[string]string

// Suffix 返回 sub 对应的邮箱后缀。
func (d EmailDomains) Suffix(sub string) (string, bool) {
	matched, found := "", false
	for prefix := range d {
		if !strings.HasPrefix(sub, prefix) {
			continue
		}
		if !found || len(prefix) > len(matched) {
			matched, found = prefix, true
		}
	}
	if !found {
		return "", false
	}
	return d[matched], true
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
			SiteTitle:      "广州大学",
			SigningKeyPath: "./data/signing_key.pem",
			LoginPanel: LoginPanel{
				Enabled: true,
				Title:   "统一身份认证门户",
				Body:    "此处为预留说明区域，可在 config.yaml 的 server.login_panel 中修改文案，或将 enabled 设为 false 隐藏本区域。",
			},
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
	if len(c.Server.EmailDomains) == 0 {
		missing = append(missing, "server.email_domains")
	}
	if len(missing) > 0 {
		return errors.New("缺少必需项: " + strings.Join(missing, ", "))
	}

	for prefix, suffix := range c.Server.EmailDomains {
		if suffix == "" {
			return fmt.Errorf("server.email_domains[%q] 的后缀不能为空", prefix)
		}
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
