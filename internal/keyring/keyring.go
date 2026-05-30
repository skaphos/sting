// SPDX-License-Identifier: MIT

// Package keyring is a thin wrapper around github.com/zalando/go-keyring
// that adds timeouts (matching the pattern used by the official GitHub CLI)
// and provides mock helpers for testing.
//
// This package exists so Sting can follow gh's exact storage standards and
// testability approach without pulling in the entire gh codebase.
package keyring

import (
	"errors"
	"fmt"
	"time"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secret not found in keyring")

type TimeoutError struct {
	message string
}

func (e *TimeoutError) Error() string {
	return e.message
}

// Set stores a secret in the keyring for the given service and user.
// It times out after 3 seconds to avoid hanging on unresponsive keyring backends.
func Set(service, user, secret string) error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		ch <- keyring.Set(service, user, secret)
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		return &TimeoutError{"timeout while trying to set secret in keyring"}
	}
}

// Get retrieves a secret from the keyring.
// It times out after 3 seconds.
// Returns ErrNotFound (wrapping the underlying not-found error) when the secret does not exist.
func Get(service, user string) (string, error) {
	ch := make(chan struct {
		val string
		err error
	}, 1)
	go func() {
		defer close(ch)
		val, err := keyring.Get(service, user)
		ch <- struct {
			val string
			err error
		}{val, err}
	}()
	select {
	case res := <-ch:
		if errors.Is(res.err, keyring.ErrNotFound) {
			// Wrap so callers can match both our sentinel
			// (errors.Is(err, ErrNotFound)) and the underlying keyring error,
			// as the doc comment promises.
			return "", fmt.Errorf("%w: %w", ErrNotFound, res.err)
		}
		return res.val, res.err
	case <-time.After(3 * time.Second):
		return "", &TimeoutError{"timeout while trying to get secret from keyring"}
	}
}

// Delete removes a secret from the keyring.
// It times out after 3 seconds.
func Delete(service, user string) error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		ch <- keyring.Delete(service, user)
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		return &TimeoutError{"timeout while trying to delete secret from keyring"}
	}
}

// MockInit initializes the underlying keyring with an in-memory mock.
// Use this in tests.
func MockInit() {
	keyring.MockInit()
}

// MockInitWithError initializes the underlying keyring mock to always return the given error.
// Useful for testing error paths.
func MockInitWithError(err error) {
	keyring.MockInitWithError(err)
}
