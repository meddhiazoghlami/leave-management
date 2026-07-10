package cli

import (
	"fmt"
	"log"

	"github.com/dzovi/leave-management/assets"
	"github.com/dzovi/leave-management/internal/handlers"
	"github.com/dzovi/leave-management/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the HTTP server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		st, cfg, err := openStore(ctx)
		if err != nil {
			return err
		}
		defer st.Close()

		// Best-effort cleanup of expired sessions on boot.
		_ = st.DeleteExpiredSessions(ctx)

		// Phase 8: dev mode (VITE_DEV=true) points the layout at the Vite dev
		// server for HMR; otherwise we read the built manifest.
		if err := assets.Init(cfg.ViteDev); err != nil {
			return fmt.Errorf("load asset manifest (did you run `npm run build` in web/?): %w", err)
		}

		h := handlers.New(st, cfg)
		r := server.New(h, st)

		log.Printf("listening on %s", cfg.Addr)
		return r.Run(cfg.Addr)
	},
}
