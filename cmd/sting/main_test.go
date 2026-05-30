// SPDX-License-Identifier: MIT
package main

import (
	"os"
	"testing"
)

// TestMain_Version runs the thin entrypoint via the `version` subcommand, which
// returns nil so cli.Execute does not call os.Exit. HOME is redirected to a
// temp dir so config loading never touches the real environment.
func TestMain_Version(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })

	t.Setenv("HOME", t.TempDir())
	os.Args = []string{"sting", "version"}

	main()
}
