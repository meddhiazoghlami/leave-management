// Package bootstrap seeds the two operational accounts (admin + HR) the app
// needs to be usable on a fresh database. Unlike internal/seed (demo data for
// local play), this runs on every `serve` startup and is production-oriented:
// each account gets a cryptographically-random password that is emailed, never
// hardcoded, and existing accounts are left completely untouched.
package bootstrap

import (
	"context"
	"fmt"
	"log"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/db"
)

// Store is the slice of the data layer bootstrap needs. Declared consumer-side
// (like auth.SessionStore) so bootstrap depends on two methods, not the whole
// *store.Store.
type Store interface {
	GetEmployeeByEmail(ctx context.Context, email string) (db.Employee, error)
	CreateEmployee(ctx context.Context, name, email, passwordHash, role string, managerID *int64) (db.Employee, error)
}

// Mailer sends the credential email. Consumer-side interface so any transport
// (SMTP, a fake in tests) satisfies it.
type Mailer interface {
	Send(to, subject, body string) error
}

// Options are the addresses to bootstrap and the base URL used in the email.
type Options struct {
	AdminEmail string
	HREmail    string
	BaseURL    string
}

// Run ensures an admin and an HR account exist. For each configured email with
// no matching account, it generates a password, emails it, and only then
// persists the account — so a send failure aborts before any user is left with
// a password nobody knows. Accounts that already exist are skipped (no email,
// no change).
//
// newMailer is called lazily the first time a message must actually be sent, so
// a deployment whose accounts already exist never requires SMTP to be
// configured. Any error (missing SMTP config, failed send, DB error) is returned
// so the caller can abort startup.
func Run(ctx context.Context, st Store, opts Options, newMailer func() (Mailer, error)) error {
	targets := []struct {
		email, name, role string
	}{
		{opts.AdminEmail, "Administrator", auth.RoleAdmin},
		{opts.HREmail, "HR", auth.RoleHR},
	}

	var mailer Mailer // built on first use via newMailer
	for _, t := range targets {
		if t.email == "" {
			log.Printf("bootstrap: no email configured for %s — skipping", t.role)
			continue
		}
		if _, err := st.GetEmployeeByEmail(ctx, t.email); err == nil {
			log.Printf("bootstrap: %s account %s already exists — skipping", t.role, t.email)
			continue
		}

		if mailer == nil {
			m, err := newMailer()
			if err != nil {
				return fmt.Errorf("bootstrap: mail transport required to create %s account: %w", t.role, err)
			}
			mailer = m
		}

		password, err := auth.GeneratePassword(0)
		if err != nil {
			return fmt.Errorf("bootstrap: generate password for %s: %w", t.email, err)
		}

		// Send first: if delivery fails we abort without ever persisting an
		// account whose password no one knows.
		subject, body := credentialEmail(t.name, t.role, t.email, password, opts.BaseURL)
		if err := mailer.Send(t.email, subject, body); err != nil {
			return fmt.Errorf("bootstrap: email credentials to %s: %w", t.email, err)
		}

		hash, err := auth.HashPassword(password)
		if err != nil {
			return fmt.Errorf("bootstrap: hash password for %s: %w", t.email, err)
		}
		if _, err := st.CreateEmployee(ctx, t.name, t.email, hash, t.role, nil); err != nil {
			return fmt.Errorf("bootstrap: create %s account %s: %w", t.role, t.email, err)
		}
		log.Printf("bootstrap: created %s account %s and emailed its password", t.role, t.email)
	}
	return nil
}

// credentialEmail builds the subject and plain-text body for a new account's
// welcome mail.
func credentialEmail(name, role, email, password, baseURL string) (subject, body string) {
	subject = "Your Leave Management account"
	body = fmt.Sprintf(`Hi %s,

An account has been created for you on Leave Management with the %q role.

    URL:      %s/login
    Email:    %s
    Password: %s

Please sign in and keep this password somewhere safe.
`, name, role, baseURL, email, password)
	return subject, body
}
