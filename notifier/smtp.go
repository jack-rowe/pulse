package notifier

import (
	"fmt"
	"net/smtp"
	"strings"
)

// SMTP sends email notifications via SMTP.
type SMTP struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
}

// NewSMTP creates an SMTP email notifier.
func NewSMTP(host string, port int, username, password, from string, to []string) *SMTP {
	return &SMTP{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		to:       to,
	}
}

func (s *SMTP) Name() string { return "smtp" }

func (s *SMTP) Notify(event Event) error {
	subject := fmt.Sprintf("Pulse Alert: %s is %s", event.EndpointName, event.NewStatus)
	body := FormatMessage(event)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		s.from,
		strings.Join(s.to, ", "),
		subject,
		body,
	)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	if err := smtp.SendMail(addr, auth, s.from, s.to, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
