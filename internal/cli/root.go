// Package cli defines the Cobra command tree for the single leave-management
// binary: `serve` runs the HTTP server, `seed` populates demo data. main.go is
// a thin wrapper around Execute.
package cli

import (
	"context"

	"github.com/dzovi/leave-management/internal/config"
	"github.com/dzovi/leave-management/internal/store"
	"github.com/spf13/cobra"
)

// version is overridable at build time: -ldflags "-X ...cli.version=v1.2.3".
var version = "dev"

var rootCmd = &cobra.Command{
	Use:           "leave-management",
	Short:         "Leave management — HTTP server and admin commands",
	Version:       version,
	SilenceUsage:  true, // don't dump usage on runtime errors, only on bad flags
	SilenceErrors: false,
}

// Execute runs the CLI. Returns an error so main can set the exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(serveCmd, seedCmd)
}

// openStore loads configuration (env / .env) and opens the connection pool —
// the shared prologue for every command that touches the database.
func openStore(ctx context.Context) (*store.Store, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, err
	}
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, config.Config{}, err
	}
	return st, cfg, nil
}
