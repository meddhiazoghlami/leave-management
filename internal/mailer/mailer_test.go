package mailer

import (
	"strings"
	"testing"
)

func TestNewSMTP_RequiresCoreSettings(t *testing.T) {
	if _, err := NewSMTP("", "587", "", "", "no-reply@x.test"); err == nil {
		t.Error("expected error when host is missing")
	}
	if _, err := NewSMTP("smtp.x.test", "587", "", "", ""); err == nil {
		t.Error("expected error when from is missing")
	}
	// username/password are optional.
	if _, err := NewSMTP("smtp.x.test", "587", "", "", "no-reply@x.test"); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("from@x.test", "to@y.test", "Hi", "line one\nline two"))

	for _, want := range []string{
		"From: from@x.test",
		"To: to@y.test",
		"Subject: Hi",
		`Content-Type: text/plain; charset="UTF-8"`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing header %q\n---\n%s", want, msg)
		}
	}

	// Header block ends with a blank line before the body.
	if !strings.Contains(msg, "\r\n\r\nline one") {
		t.Error("expected blank line separating headers from body")
	}
	// Body newlines are normalised to CRLF.
	if !strings.Contains(msg, "line one\r\nline two") {
		t.Error("body newlines not CRLF-normalised")
	}
}
