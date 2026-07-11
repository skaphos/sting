// SPDX-License-Identifier: MIT

package credentials

// Test-only construction seams. These live in an _test.go file so they never
// ship in production binaries, yet remain in-package so tests can inject a fake
// keyring backend and a deterministic file path.

// WithFilePath returns a file-only Store that uses a specific directory for
// plaintext storage. It never consults the system keyring and never touches
// GH_CONFIG_DIR, so test behavior is deterministic regardless of whether a real
// keyring is available on the host.
func WithFilePath(dir string) Store {
	return newStore(dir, nil)
}

// WithKeyringForTest returns a Store that uses the provided KeyringBackend for
// the secure path. A nil backend selects file-only mode (no keyring), matching
// WithFilePath. The insecure path uses our own file-based storage under the
// given directory.
func WithKeyringForTest(backend KeyringBackend, configDir string) Store {
	return newStore(configDir, backend)
}
