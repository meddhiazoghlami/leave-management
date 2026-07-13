package main

import (
	"os"
	"testing"
)

// TestMain_HelpReturnsCleanly exercises the entrypoint: `--help` makes the CLI
// return nil, so main falls through without calling os.Exit. (The os.Exit(1)
// error branch can't be covered without terminating the test process.)
func TestMain_HelpReturnsCleanly(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"leave-management", "--help"}

	main()
}
