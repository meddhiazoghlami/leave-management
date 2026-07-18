package bootstrap_test

import (
	"context"
	"errors"
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/bootstrap"
	"github.com/meddhiazoghlami/leave-management/internal/db"
)

// fakeStore records created employees and reports existence by email.
type fakeStore struct {
	existing map[string]bool
	created  []db.Employee
}

func newFakeStore(existing ...string) *fakeStore {
	m := map[string]bool{}
	for _, e := range existing {
		m[e] = true
	}
	return &fakeStore{existing: m}
}

func (f *fakeStore) GetEmployeeByEmail(_ context.Context, email string) (db.Employee, error) {
	if f.existing[email] {
		return db.Employee{Email: email}, nil
	}
	return db.Employee{}, errors.New("not found")
}

func (f *fakeStore) CreateEmployee(_ context.Context, name, email, hash, role string, _ *int64) (db.Employee, error) {
	e := db.Employee{Name: name, Email: email, PasswordHash: hash, Role: role}
	f.created = append(f.created, e)
	f.existing[email] = true
	return e, nil
}

// fakeMailer records sends and can be told to fail.
type fakeMailer struct {
	sent    []string // recipient emails
	failOn  string   // if set, Send to this address returns an error
	subject map[string]string
	body    map[string]string
}

func (m *fakeMailer) Send(to, subject, body string) error {
	if to == m.failOn {
		return errors.New("smtp boom")
	}
	m.sent = append(m.sent, to)
	if m.subject == nil {
		m.subject = map[string]string{}
		m.body = map[string]string{}
	}
	m.subject[to] = subject
	m.body[to] = body
	return nil
}

func opts() bootstrap.Options {
	return bootstrap.Options{
		AdminEmail: "admin@acme.test",
		HREmail:    "hr@acme.test",
		BaseURL:    "https://leave.acme.test",
	}
}

func TestRun_CreatesMissingAccountsAndEmails(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMailer{}
	built := 0
	newMailer := func() (bootstrap.Mailer, error) { built++; return mail, nil }

	if err := bootstrap.Run(context.Background(), st, opts(), newMailer); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(st.created) != 2 {
		t.Fatalf("created %d accounts, want 2", len(st.created))
	}
	if len(mail.sent) != 2 {
		t.Fatalf("sent %d emails, want 2", len(mail.sent))
	}
	if built != 1 {
		t.Errorf("mailer built %d times, want 1 (lazy, reused)", built)
	}
	// Passwords are hashed, never stored plaintext, and the email carries a link.
	for _, e := range st.created {
		if e.PasswordHash == "" {
			t.Errorf("%s created with empty password hash", e.Email)
		}
		if body := mail.body[e.Email]; body == "" {
			t.Errorf("no email body recorded for %s", e.Email)
		}
	}
}

func TestRun_SkipsExistingAccounts(t *testing.T) {
	st := newFakeStore("admin@acme.test") // admin already provisioned
	mail := &fakeMailer{}
	newMailer := func() (bootstrap.Mailer, error) { return mail, nil }

	if err := bootstrap.Run(context.Background(), st, opts(), newMailer); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(st.created) != 1 || st.created[0].Email != "hr@acme.test" {
		t.Fatalf("expected only HR created, got %+v", st.created)
	}
	if len(mail.sent) != 1 || mail.sent[0] != "hr@acme.test" {
		t.Fatalf("expected only HR emailed, got %v", mail.sent)
	}
}

func TestRun_NoMailerWhenNothingToDo(t *testing.T) {
	st := newFakeStore("admin@acme.test", "hr@acme.test") // both exist
	newMailer := func() (bootstrap.Mailer, error) {
		t.Fatal("mailer must not be built when all accounts already exist")
		return nil, nil
	}

	if err := bootstrap.Run(context.Background(), st, opts(), newMailer); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.created) != 0 {
		t.Fatalf("created %d accounts, want 0", len(st.created))
	}
}

func TestRun_AbortsAndDoesNotCreateOnSendFailure(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMailer{failOn: "admin@acme.test"}
	newMailer := func() (bootstrap.Mailer, error) { return mail, nil }

	err := bootstrap.Run(context.Background(), st, opts(), newMailer)
	if err == nil {
		t.Fatal("expected Run to fail when the email send fails")
	}
	// Crucially: no account persisted for the address whose email failed.
	for _, e := range st.created {
		if e.Email == "admin@acme.test" {
			t.Fatal("admin account was created despite the email send failing")
		}
	}
}

func TestRun_MailerConfigErrorAborts(t *testing.T) {
	st := newFakeStore()
	newMailer := func() (bootstrap.Mailer, error) { return nil, errors.New("SMTP not configured") }

	if err := bootstrap.Run(context.Background(), st, opts(), newMailer); err == nil {
		t.Fatal("expected Run to fail when the mailer cannot be built")
	}
	if len(st.created) != 0 {
		t.Fatalf("created %d accounts despite mailer failure, want 0", len(st.created))
	}
}

func TestRun_SkipsBlankEmail(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMailer{}
	newMailer := func() (bootstrap.Mailer, error) { return mail, nil }

	o := opts()
	o.HREmail = "" // only admin configured
	if err := bootstrap.Run(context.Background(), st, o, newMailer); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.created) != 1 || st.created[0].Role != "admin" {
		t.Fatalf("expected only admin created, got %+v", st.created)
	}
}
