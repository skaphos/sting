// SPDX-License-Identifier: MIT

package keyring

import (
	"errors"
	"testing"
)

func TestSetGetDelete(t *testing.T) {
	MockInit()

	service := "sting-test"
	user := "test-user"
	secret := "super-secret-token"

	// Should not exist yet
	_, err := Get(service, user)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound before Set, got %v", err)
	}

	// Set
	if err := Set(service, user, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	got, err := Get(service, user)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != secret {
		t.Errorf("Get returned %q, want %q", got, secret)
	}

	// Delete
	if err := Delete(service, user); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should be gone again
	_, err = Get(service, user)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}
}

func TestGetNotFound(t *testing.T) {
	MockInit()

	_, err := Get("nonexistent-service", "nonexistent-user")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMockInitWithError(t *testing.T) {
	wantErr := errors.New("simulated keyring failure")
	MockInitWithError(wantErr)

	err := Set("svc", "user", "secret")
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("expected wrapped error %v, got %v", wantErr, err)
	}
}
