package cli

import (
	"fmt"
	"log"

	"github.com/dzovi/leave-management/assets"
	"github.com/dzovi/leave-management/internal/app"
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

		// Phase 8: dev mode (VITE_DEV=true) points the layout at the Vite dev
		// server for HMR; otherwise we read the built manifest.
		if err := assets.Init(application.Config.ViteDev); err != nil {
			return fmt.Errorf("load asset manifest (did you run `npm run build` in web/?): %w", err)
		}

		log.Printf("listening on %s", application.Config.Addr)
		return application.Router.Run(application.Config.Addr)
	},
}
