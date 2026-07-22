package cli

import (
	"fmt"

	"github.com/meddhiazoghlami/leave-management/assets"
	"github.com/meddhiazoghlami/leave-management/internal/app"
	"github.com/meddhiazoghlami/leave-management/internal/bootstrap"
	"github.com/meddhiazoghlami/leave-management/internal/mailer"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the HTTP server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		// Wire builds config → store → handlers → router; cleanup closes the pool.
		application, cleanup, err := app.InitializeApp(ctx)
		if err != nil {
			return err
		}
		defer cleanup()

		// Best-effort cleanup of expired sessions on boot.
		_ = application.Store.DeleteExpiredSessions(ctx)

		// Ensure the admin + HR accounts exist, mailing each a random password on
		// first creation. The mailer is built lazily (only if an account is
		// actually missing), so an already-provisioned deployment needs no SMTP.
		cfg := application.Config
		newMailer := func() (bootstrap.Mailer, error) {
			return mailer.NewSMTP(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
		}
		if err := bootstrap.Run(ctx, application.Store, bootstrap.Options{
			AdminEmail: cfg.BootstrapAdminEmail,
			HREmail:    cfg.BootstrapHREmail,
			BaseURL:    cfg.BaseURL,
		}, newMailer); err != nil {
			return err
		}

		// Phase 8: dev mode (VITE_DEV=true) points the layout at the Vite dev
		// server for HMR; otherwise we read the built manifest.
		if err := assets.Init(application.Config.ViteDev); err != nil {
			return fmt.Errorf("load asset manifest (did you run `npm run build` in web/?): %w", err)
		}

		application.Logger.Info("listening", "addr", application.Config.Addr)
		return application.Router.Run(application.Config.Addr)
	},
}
