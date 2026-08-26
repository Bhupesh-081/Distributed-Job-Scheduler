// Package mailer sends plain-text email over SMTP - stdlib net/smtp, no
// third-party SDK. Works against any authenticated relay (Gmail, SES,
// Brevo, Resend, etc) since net/smtp.SendMail negotiates STARTTLS on its
// own when the server offers it.
package mailer

import (
	"fmt"
	"net/smtp"
)

type Config struct {
	Host     string
	Port     string
	From     string
	User     string
	Password string
}

type Mailer struct {
	cfg Config
}

func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// Send delivers a plain-text email. A send failure is the caller's to
// decide how to handle - auth flows log and continue rather than fail the
// request, since a dead mail relay shouldn't block registration/login.
func (m *Mailer) Send(to, subject, body string) error {
	addr := m.cfg.Host + ":" + m.cfg.Port
	msg := fmt.Appendf(nil,
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		m.cfg.From, to, subject, body,
	)
	auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)
	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, msg)
}
