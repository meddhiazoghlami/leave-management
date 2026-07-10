package cli

import (
	"log"

	"github.com/dzovi/leave-management/internal/app"
	"github.com/dzovi/leave-management/internal/seed"
	"github.com/spf13/cobra"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Populate the database with demo data (idempotent)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		// DB-only injector — no HTTP router needed for seeding.
		st, cleanup, err := app.InitializeStore(ctx)
		if err != nil {
			return err
		}
		defer cleanup()

		if err := seed.Run(ctx, st); err != nil {
			return err
		}
		log.Printf("✔ seed complete — log in as admin@acme.test / manager@acme.test / sam@acme.test (password %q)", seed.Password)
		return nil
	},
}
