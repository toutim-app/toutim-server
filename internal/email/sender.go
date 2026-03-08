package email

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/fabienbmusic/toutim-server/internal/config"
)

// Sender sends emails via SMTP. When SMTP is not configured, all methods
// are no-ops that return nil.
type Sender struct {
	cfg     config.SMTPConfig
	logger  *slog.Logger
	enabled bool
}

// NewSender creates a new email sender.
func NewSender(cfg config.SMTPConfig, logger *slog.Logger) *Sender {
	return &Sender{
		cfg:     cfg,
		logger:  logger,
		enabled: cfg.Enabled(),
	}
}

// Enabled returns true if SMTP is configured.
func (s *Sender) Enabled() bool {
	return s.enabled
}

// Send sends an email with the given subject and HTML body to the recipient.
func (s *Sender) Send(to, subject, htmlBody string) error {
	if !s.enabled {
		s.logger.Warn("email not sent: SMTP not configured", "to", to, "subject", subject)
		return nil
	}

	from := s.cfg.FromAddress
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))

	// Build MIME message
	headers := [][2]string{
		{"From", s.formatFrom()},
		{"To", to},
		{"Subject", subject},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/html; charset=\"UTF-8\""},
		{"Date", time.Now().Format(time.RFC1123Z)},
	}

	var msg strings.Builder
	for _, h := range headers {
		msg.WriteString(h[0])
		msg.WriteString(": ")
		msg.WriteString(h[1])
		msg.WriteString("\r\n")
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	if s.cfg.UseTLS {
		return s.sendTLS(addr, auth, from, to, []byte(msg.String()))
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
}

// sendTLS connects with implicit TLS (for port 465).
func (s *Sender) sendTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: s.cfg.Host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}

	return client.Quit()
}

func (s *Sender) formatFrom() string {
	if s.cfg.FromName != "" {
		return fmt.Sprintf("%s <%s>", s.cfg.FromName, s.cfg.FromAddress)
	}
	return s.cfg.FromAddress
}
