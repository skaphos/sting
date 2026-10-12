// SPDX-License-Identifier: MIT
package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/model"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const reportPrefix = "sting: credential: "

// runRoot executes the real command tree (cobra parsing, OnInitialize config
// loading, and the init-time flag bindings) against a fresh viper, then
// restores every flag it may have touched so later tests see pristine state.
func runRoot(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	orig := v
	v = viper.New()
	bindPersistentFlags(v)
	t.Cleanup(func() {
		v = orig
		for _, fs := range []*pflag.FlagSet{rootCmd.PersistentFlags(), queryCmd.Flags()} {
			fs.VisitAll(func(f *pflag.Flag) {
				// Set(DefValue) on a slice flag would store the literal "[]".
				if sv, ok := f.Value.(pflag.SliceValue); ok {
					_ = sv.Replace(nil)
				} else {
					_ = f.Value.Set(f.DefValue)
				}
				f.Changed = false
			})
		}
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	var out, errBuf bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs(args)
	// The fixture server returns no commits, so the run may still fail after
	// the client is built; the report is written at construction, so log rather
	// than fail, keeping the cause visible if a case stops earlier.
	if err := rootCmd.Execute(); err != nil {
		t.Logf("execute: %v", err)
	}
	return out.String(), errBuf.String()
}

// TestVerboseEnabledThroughEverySurface covers FR-004 end to end: the parsed
// --verbose flag, STING_VERBOSE, and `verbose: true` in a config file each turn
// the report on through the real binding and config-loading path.
func TestVerboseEnabledThroughEverySurface(t *testing.T) {
	srv := commitServer(t)
	query := []string{"query", "--author", "alice", "--scope", "repos", "--repos", "a/b", "-o", "json"}
	want := reportPrefix + "github 127.0.0.1: dedicated PAT\n"

	cases := map[string]func(t *testing.T) []string{
		"flag": func(*testing.T) []string {
			return append(query, "--verbose", "--token", "fixture-secret", "--base-url", srv.URL+"/")
		},
		"env": func(t *testing.T) []string {
			t.Setenv("STING_VERBOSE", "true")
			return append(query, "--token", "fixture-secret", "--base-url", srv.URL+"/")
		},
		"config file": func(t *testing.T) []string {
			path := filepath.Join(t.TempDir(), "config.yaml")
			body := "verbose: true\ntoken: fixture-secret\nbase_url: " + srv.URL + "/\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			return append(query, "--config", path)
		},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			isolateHome(t)
			stdout, stderr := runRoot(t, args(t)...)
			if stderr != want {
				t.Fatalf("stderr = %q, want %q", stderr, want)
			}
			if strings.Contains(stdout, reportPrefix) || strings.Contains(stdout+stderr, "fixture-secret") {
				t.Fatalf("report or secret leaked: stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}

	t.Run("off by default", func(t *testing.T) {
		isolateHome(t)
		_, stderr := runRoot(t, append(query, "--token", "fixture-secret", "--base-url", srv.URL+"/")...)
		if stderr != "" {
			t.Fatalf("non-verbose stderr must be empty, got %q", stderr)
		}
	})
}

// TestDiagnosticsGate covers the three ways to enable verbosity and the
// default: off must yield a nil writer so constructors stay silent.
func TestDiagnosticsGate(t *testing.T) {
	seedValidConfig(t)
	cmd, _, errBuf := newCmd()

	if w := diagnostics(cmd); w != nil {
		t.Fatalf("verbose off by default: want nil writer, got %T", w)
	}

	v.Set("verbose", true)
	if w := diagnostics(cmd); w != errBuf {
		t.Fatalf("verbose on: want the command's stderr, got %T", w)
	}

	seedValidConfig(t)
	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()
	t.Setenv("STING_VERBOSE", "true")
	if w := diagnostics(cmd); w != errBuf {
		t.Fatal("STING_VERBOSE=true must enable diagnostics")
	}
}

func commitServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestQueryReportsCredentialOnlyWhenVerbose runs the real query path against
// httptest: the report lands on stderr when verbose and nowhere otherwise, and
// stdout (the evidence) never carries it or the secret.
func TestQueryReportsCredentialOnlyWhenVerbose(t *testing.T) {
	isolateHome(t)
	srv := commitServer(t)

	for _, verbose := range []bool{true, false} {
		seedValidConfig(t)
		v.Set("base_url", srv.URL+"/")
		v.Set("token", "fixture-secret")
		v.Set("verbose", verbose)

		cmd, out, errBuf := newCmd()
		cmd.SetContext(context.Background())
		registerQueryFlags(cmd)
		for name, value := range map[string]string{"author": "alice", "scope": "repos", "repos": "a/b", "format": "json"} {
			if err := cmd.Flags().Set(name, value); err != nil {
				t.Fatal(err)
			}
		}
		_ = runQuery(cmd, nil) // the report is written at client construction, before collection

		if strings.Contains(out.String(), reportPrefix) || strings.Contains(out.String()+errBuf.String(), "fixture-secret") {
			t.Fatalf("verbose=%v: report or secret leaked: stdout=%q stderr=%q", verbose, out, errBuf)
		}
		want := reportPrefix + "github 127.0.0.1: dedicated PAT\n"
		if verbose && errBuf.String() != want {
			t.Fatalf("verbose stderr = %q, want %q", errBuf, want)
		}
		if !verbose && errBuf.Len() != 0 {
			t.Fatalf("non-verbose stderr must be empty, got %q", errBuf)
		}
	}
}

func TestActivityReportsCredentialWhenVerbose(t *testing.T) {
	isolateHome(t)
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)
	v.Set("verbose", true)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	_ = runActivityCmd(cmd, "--repo", "o/r")
	if !strings.HasPrefix(errBuf.String(), reportPrefix+"github 127.0.0.1: dedicated PAT") {
		t.Fatalf("activity stderr = %q, want a credential report", errBuf.String())
	}
}

// TestPRCommandsPassTheGatedWriter confirms prs and inbox hand their verbose
// writer to the client constructor; the seams build a real PR client (no
// network at construction) so the report is produced by commitclient itself.
func TestPRCommandsPassTheGatedWriter(t *testing.T) {
	isolateHome(t)
	origActivity, origInbox := collectPRActivity, collectPRInbox
	t.Cleanup(func() { collectPRActivity, collectPRInbox = origActivity, origInbox })
	collectPRActivity = func(_ context.Context, cfg config.Config, q model.PRQuery, opts ...commitclient.Option) (model.PRResult, error) {
		_, err := commitclient.NewPR(cfg, opts...)
		return model.PRResult{SchemaVersion: model.PRSchemaVersion, Query: q, PRs: []model.PRActivityEntry{}}, err
	}
	collectPRInbox = func(_ context.Context, cfg config.Config, q model.PRInboxQuery, opts ...commitclient.Option) (model.PRInboxResult, error) {
		_, err := commitclient.NewPR(cfg, opts...)
		return model.PRInboxResult{SchemaVersion: model.PRInboxSchemaVersion, Query: q}, err
	}

	run := map[string]func() (*bytes.Buffer, error){
		"prs": func() (*bytes.Buffer, error) {
			cmd, _, errBuf := newCmd()
			cmd.SetContext(context.Background())
			registerPRFlags(cmd)
			if err := cmd.ParseFlags([]string{"--author", "octocat", "--format", "json"}); err != nil {
				return nil, err
			}
			return errBuf, runPRs(cmd, nil)
		},
		"inbox": func() (*bytes.Buffer, error) {
			cmd, _, errBuf := newCmd()
			cmd.SetContext(context.Background())
			registerInboxFlags(cmd)
			if err := cmd.ParseFlags([]string{"--user", "octocat", "--format", "json"}); err != nil {
				return nil, err
			}
			return errBuf, runInbox(cmd, nil)
		},
	}
	for name, fn := range run {
		t.Run(name, func(t *testing.T) {
			seedValidConfig(t)
			v.Set("token", "fixture-secret")
			v.Set("verbose", true)
			errBuf, err := fn()
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if want := reportPrefix + "github github.com: dedicated PAT\n"; errBuf.String() != want {
				t.Fatalf("stderr = %q, want %q", errBuf, want)
			}
		})
	}
}
