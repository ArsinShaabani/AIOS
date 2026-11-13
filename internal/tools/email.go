package tools

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// SMTPConfig holds outbound mail settings (password comes from the encrypted
// secret store, never from plain config files).
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
}

// SMTP sends mail through a configured relay.
type SMTP struct {
	cfg SMTPConfig
}

// NewSMTP builds a sender; returns nil when the host is not configured.
func NewSMTP(cfg SMTPConfig) *SMTP {
	if cfg.Host == "" || cfg.From == "" {
		return nil
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.Username == "" {
		cfg.Username = cfg.From
	}
	return &SMTP{cfg: cfg}
}

// Send delivers a plain-text email.
func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	if strings.TrimSpace(to) == "" || !strings.Contains(to, "@") {
		return platform.Invalid("email.to", "a valid recipient is required")
	}
	if len(body) > 500_000 {
		return platform.Invalid("email.body", "email body is too large")
	}
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	msg := buildMessage(s.cfg.From, to, subject, body)
	if s.cfg.UseTLS {
		// Implicit TLS (port 465).
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * 1e9}, "tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return platform.Wrap(platform.KindUnavailable, "email.dial", "cannot connect to SMTP server", err)
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			return platform.Wrap(platform.KindUnavailable, "email.client", "SMTP handshake failed", err)
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return platform.Wrap(platform.KindUnauthorized, "email.auth", "SMTP authentication failed", err)
		}
		if err := client.Mail(s.cfg.From); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write(msg); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}
	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{to}, msg); err != nil {
		return platform.Wrap(platform.KindUnavailable, "email.send", "cannot send email", err)
	}
	return nil
}

func buildMessage(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + sanitizeHeader(subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return platform.Truncate(strings.TrimSpace(s), 200)
}

// RegisterEmailTool registers send_email.
func RegisterEmailTool(reg *Registry, sender EmailSender) error {
	if sender == nil {
		return nil
	}
	return reg.Register(ports.Tool{
		Name:        "send_email",
		Description: "Send a plain-text email. Only use when the user explicitly asks to send something.",
		Parameters: Schema(map[string]any{
			"to":      map[string]any{"type": "string", "description": "Recipient email address"},
			"subject": map[string]any{"type": "string", "description": "Subject line"},
			"body":    map[string]any{"type": "string", "description": "Plain-text body"},
		}, "to", "subject", "body"),
		Permissions: []string{"chat"},
		Timeout:     30,
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			to := stringArg(call.Arguments, "to")
			subject := stringArg(call.Arguments, "subject")
			body := stringArg(call.Arguments, "body")
			if to == "" || subject == "" || body == "" {
				return ports.ToolResult{IsError: true, Content: "to, subject and body are required"}, nil
			}
			if err := sender.Send(ctx, to, subject, body); err != nil {
				return ports.ToolResult{IsError: true, Content: "email failed: " + err.Error()}, nil
			}
			return ports.ToolResult{Content: "email sent to " + to}, nil
		},
	})
}
