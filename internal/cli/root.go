// Package cli defines the Cobra command tree for the single leave-management
// binary: `serve` runs the HTTP server, `seed` populates demo data. Dependencies
// are built by Wire (see internal/app); the commands just call the injectors.
package cli

import "github.com/spf13/cobra"

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
