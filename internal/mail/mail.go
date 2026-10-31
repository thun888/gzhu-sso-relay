package mail

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type Sender struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSender(cfg Config) *Sender {
	return &Sender{host: cfg.Host, port: cfg.Port, username: cfg.Username, password: cfg.Password, from: cfg.From}
}

func (s *Sender) Enabled() bool {
	return s != nil && s.host != "" && s.from != ""
}

func (s *Sender) Send(to, subject, body string) error {
	if !s.Enabled() {
		return nil
	}

	addr := net.JoinHostPort(s.host, fmt.Sprint(s.port))
	conn, err := s.dial(addr)
	if err != nil {
		return fmt.Errorf("mail: 连接 %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mail: 握手 %s: %w", addr, err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return fmt.Errorf("mail: STARTTLS: %w", err)
		}
	}
	if s.username != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mail: 认证: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(s.from, to, subject, body)); err != nil {
		w.Close()
		return fmt.Errorf("mail: 写入正文: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: 结束正文: %w", err)
	}
	return client.Quit()
}

func (s *Sender) dial(addr string) (net.Conn, error) {
	if s.port == 465 {
		d := &net.Dialer{Timeout: 15 * time.Second}
		return tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: s.host})
	}
	return net.DialTimeout("tcp", addr, 15*time.Second)
}

func buildMessage(from, to, subject, body string) []byte {
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var wrapped strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := min(i+76, len(encoded))
		wrapped.WriteString(encoded[i:end])
		wrapped.WriteString("\r\n")
	}
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: base64",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + wrapped.String())
}
