// Command leave-management is the entrypoint — a thin wrapper around the Cobra
// CLI. Subcommands: `serve` (HTTP server) and `seed` (demo data). All real logic
// lives under internal/.
package main

import (
	"os"

	"github.com/dzovi/leave-management/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// Cobra already printed the error; just set a non-zero exit code.
		os.Exit(1)
	}
}
