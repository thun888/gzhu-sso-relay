package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/thun888/gzhu-sso-relay/internal/binding"
)

type Config struct {
	Server    Server   `yaml:"server"`
	SecretKey string   `yaml:"secret_key"`
	Database  Database `yaml:"database"`
	Redis     Redis    `yaml:"redis"`
	Upstream  Upstream `yaml:"upstream"`
	Captcha   Captcha  `yaml:"captcha"`
	Bindings  Bindings `yaml:"bindings"`
}

// Captcha 极验行为验证。fail_window 内有失败记录时该 IP 登录需先过验证码，
// always 为 true 时每次登录都要验证，且不再记录失败次数。
type Captcha struct {
	Enabled    bool     `yaml:"enabled"`
	Always     bool     `yaml:"always"`
	CaptchaID  string   `yaml:"captcha_id"`
	CaptchaKey string   `yaml:"captcha_key"`
	FailWindow Duration `yaml:"fail_window"`
}

// Bindings 声明可绑定的第三方账号来源，每个来源走标准 OAuth2 授权码流程。
type Bindings struct {
	Providers []binding.Provider `yaml:"providers"`
}

type Server struct {
	Addr           string       `yaml:"addr"`
	BaseURL        string       `yaml:"base_url"`
	LoginURL       string       `yaml:"login_url"`
	ConsentURL     string       `yaml:"consent_url"`
	AccountURL     string       `yaml:"account_url"`
	SiteTitle      string       `yaml:"site_title"`
	EmailDomains   EmailDomains `yaml:"email_domains"`
	SigningKeyPath string       `yaml:"signing_key_path"`
	LogoDir        string       `yaml:"logo_dir"`
	TrustedProxies []string     `yaml:"trusted_proxies"`
	LoginPanel     LoginPanel   `yaml:"login_panel"`
	LoginFooter    LoginFooter  `yaml:"login_footer"`
}

// LoginPanel 控制登录页右半区的预留文案，enabled 为 false 时登录卡片收成单栏。
type LoginPanel struct {
	Enabled bool   `yaml:"enabled"`
	Title   string `yaml:"title"`
	Body    string `yaml:"body"`
}

// LoginFooter 控制登录页卡片下方的页脚文案，enabled 为 false 或文案为空时不显示。
type LoginFooter struct {
	Enabled bool   `yaml:"enabled"`
	Text    string `yaml:"text"`
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
	// RefreshRate 是每次登录成功后异步回访上游的概率，0 表示从不刷新。
	RefreshRate float64 `yaml:"refresh_rate"`
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
			ConsentURL:     "/consent",
			AccountURL:     "/",
			SiteTitle:      "广州大学",
			SigningKeyPath: "./data/signing_key.pem",
			LogoDir:        "./data/logos",
			TrustedProxies: []string{"127.0.0.1", "::1"},
			LoginPanel: LoginPanel{
				Enabled: true,
				Title:   "统一身份认证门户",
				Body:    "此处为预留说明区域，可在 config.yaml 的 server.login_panel 中修改文案，或将 enabled 设为 false 隐藏本区域。",
			},
			LoginFooter: LoginFooter{
				Enabled: true,
				Text:    "© 广州大学",
			},
		},
		Redis: Redis{Addr: "localhost:6379"},
		Captcha: Captcha{
			FailWindow: Duration(24 * time.Hour),
		},
		Upstream: Upstream{
			CASBaseURL:    "https://newcas.gzhu.edu.cn",
			PortalBaseURL: "https://newmy.gzhu.edu.cn",
			Timeout:       Duration(30 * time.Second),
			RefreshRate:   0.1,
		},
	}
}

func (c *Config) normalize() {
	c.Server.BaseURL = strings.TrimSuffix(c.Server.BaseURL, "/")
	if c.Server.LoginURL == "" {
		c.Server.LoginURL = "/login"
	}
	if c.Server.LogoDir == "" {
		c.Server.LogoDir = "./data/logos"
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
	if c.Captcha.Enabled {
		if c.Captcha.CaptchaID == "" {
			missing = append(missing, "captcha.captcha_id")
		}
		if c.Captcha.CaptchaKey == "" {
			missing = append(missing, "captcha.captcha_key")
		}
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
	if c.Upstream.RefreshRate < 0 || c.Upstream.RefreshRate > 1 {
		return errors.New("upstream.refresh_rate 必须在 0 到 1 之间")
	}
	if c.Captcha.FailWindow <= 0 {
		return errors.New("captcha.fail_window 必须为正数")
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
