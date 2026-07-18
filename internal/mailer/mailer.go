// Package mailer is the outbound email transport. It exposes a tiny Send
// interface and one implementation over the standard library's net/smtp, so the
// rest of the app can send mail without importing SMTP details. Keeping the
// message assembly (buildMessage) separate from the network send keeps the
// header/MIME formatting unit-testable without a live SMTP server.
package mailer

import (
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// SMTP sends mail through an SMTP relay using LOGIN/PLAIN auth. It's safe to
// construct once and reuse; net/smtp opens a fresh connection per Send.
type SMTP struct {
	addr     string // host:port
	host     string // bare host, for PlainAuth's server-name check
	username string
	password string
	from     string
}

// NewSMTP validates the SMTP settings and returns a ready sender. host, port and
// from are mandatory; username/password may be empty for relays that don't
// authenticate (in which case no AUTH is attempted). A missing required field is
// a hard error so a misconfigured deployment fails loudly rather than silently
// dropping mail.
func NewSMTP(host, port, username, password, from string) (*SMTP, error) {
	var missing []string
	if host == "" {
		missing = append(missing, "SMTP_HOST")
	}
	if port == "" {
		missing = append(missing, "SMTP_PORT")
	}
	if from == "" {
		missing = append(missing, "SMTP_FROM")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("mailer: missing required SMTP settings: %s", strings.Join(missing, ", "))
	}
	return &SMTP{
		addr:     net.JoinHostPort(host, port),
		host:     host,
		username: username,
		password: password,
		from:     from,
	}, nil
}

// Send delivers a plain-text message to a single recipient.
func (m *SMTP) Send(to, subject, body string) error {
	if to == "" {
		return errors.New("mailer: empty recipient")
	}
	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}
	msg := buildMessage(m.from, to, subject, body)
	if err := smtp.SendMail(m.addr, auth, m.from, []string{to}, msg); err != nil {
		return fmt.Errorf("mailer: send to %s: %w", to, err)
	}
	return nil
}

// buildMessage assembles an RFC 5322 plain-text message. Lines are CRLF-joined
// as SMTP requires; the header block is separated from the body by a blank line.
func buildMessage(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		`Content-Type: text/plain; charset="UTF-8"`,
	}
	// Normalise body newlines to CRLF so the message is well-formed on the wire.
	normalised := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + normalised)
}
