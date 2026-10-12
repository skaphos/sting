// SPDX-License-Identifier: MIT
package commitclient

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/skaphos/sting/internal/credentials"
	"github.com/skaphos/sting/model"
)

// Option configures a client constructor.
type Option func(*options)

type options struct {
	report io.Writer
}

// WithCredentialReport makes the constructor write one line naming the
// credential source it selected (never the credential itself) to w. A nil w
// keeps the constructor silent, so callers can pass their verbosity-gated
// writer unconditionally.
func WithCredentialReport(w io.Writer) Option {
	return func(o *options) { o.report = w }
}

func applyOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func (o options) emit(r resolution) {
	if o.report != nil {
		fmt.Fprintln(o.report, r)
	}
}

// credSource is the precedence tier a resolution landed on.
type credSource string

const (
	sourceDedicated  credSource = "dedicated"
	sourceKeyring    credSource = "keyring"
	sourceFile       credSource = "file"
	sourceAmbientEnv credSource = "ambient-env"
	sourceAmbientGH  credSource = "ambient-gh"
	sourceAnonymous  credSource = "anonymous"
)

// resolution records which credential a constructor selected. It deliberately
// has no token field: a value that cannot hold the secret cannot leak it.
type resolution struct {
	provider model.Provider
	host     string
	source   credSource
	// kind is the stored credential's type; empty for dedicated and anonymous.
	kind credentials.TokenType
	// lookupReason explains why a stored-credential lookup failed, when that
	// failure is what left the resolution anonymous. It must never carry
	// credential-file contents.
	lookupReason string
}

// String renders the stderr diagnostic line (contracts/credential-report.md).
func (r resolution) String() string {
	return fmt.Sprintf("sting: credential: %s %s: %s", r.provider, r.host, r.describe())
}

func (r resolution) describe() string {
	switch r.source {
	case sourceDedicated:
		return "dedicated PAT"
	case sourceKeyring:
		return "stored " + kindLabel(r.kind) + " (keyring)"
	case sourceFile:
		return "stored " + kindLabel(r.kind) + " (file)"
	case sourceAmbientEnv:
		return "ambient token from environment (opt-in)"
	case sourceAmbientGH:
		return "ambient token from gh config (opt-in)"
	default:
		if r.lookupReason != "" {
			return "anonymous (stored-credential lookup failed: " + r.lookupReason + ")"
		}
		return "anonymous"
	}
}

// singleLine flattens an error message from the keyring/OS so a report stays
// exactly one line: an embedded \n or \r would otherwise split it, or forge a
// second, legitimate-looking "sting: credential:" line on stderr.
func singleLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func kindLabel(k credentials.TokenType) string {
	if k == credentials.TokenTypePAT {
		return "pat"
	}
	return "oauth token"
}

// storedSource maps the credential store's Source onto a report tier. The
// ambient sources are only reachable under STING_ALLOW_AMBIENT_GITHUB_TOKEN,
// which is why they are labelled opt-in rather than as sting credentials.
func storedSource(src credentials.Source) credSource {
	switch src {
	case credentials.SourceKeyring:
		return sourceKeyring
	case credentials.SourceEnv:
		return sourceAmbientEnv
	case credentials.SourceConfig:
		return sourceAmbientGH
	default:
		return sourceFile
	}
}
