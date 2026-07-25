// SPDX-License-Identifier: MIT

package buildinfo

import (
	"runtime/debug"
	"testing"
)

// bi builds a *debug.BuildInfo with the given module version and VCS settings,
// mirroring what the toolchain records for each kind of build.
func bi(mainVersion string, settings map[string]string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	info.Main.Version = mainVersion
	for k, v := range settings {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: k, Value: v})
	}
	return info
}

// TestResolvePrecedence covers every row of the resolution table: which source
// wins, and what each one reports.
func TestResolvePrecedence(t *testing.T) {
	tests := []struct {
		name                        string
		ldVersion, ldCommit, ldDate string
		info                        *debug.BuildInfo
		ok                          bool
		want                        Info
	}{
		{
			name:      "ldflags win over build metadata",
			ldVersion: "v0.7.0", ldCommit: "abc123", ldDate: "2026-07-25T10:00:00Z",
			info: bi("v9.9.9", map[string]string{"vcs.revision": "deadbeef"}),
			ok:   true,
			want: Info{
				Version: "v0.7.0", Revision: "abc123", Time: "2026-07-25T10:00:00Z",
				Source: SourceLDFlags,
			},
		},
		{
			name:      "ldflags sentinels map back to not-recorded",
			ldVersion: "v0.7.0", ldCommit: "none", ldDate: "unknown",
			info: nil, ok: false,
			want: Info{Version: "v0.7.0", Source: SourceLDFlags},
		},
		{
			name:      "module proxy install: version, no revision",
			ldVersion: "dev", ldCommit: "none", ldDate: "unknown",
			info: bi("v0.7.0", nil),
			ok:   true,
			want: Info{Version: "v0.7.0", Source: SourceBuildInfo},
		},
		{
			name:      "module proxy install with vcs stamps uses both",
			ldVersion: "dev",
			info:      bi("v0.7.0", map[string]string{"vcs.revision": "cafe", "vcs.time": "2026-01-01T00:00:00Z"}),
			ok:        true,
			want: Info{
				Version: "v0.7.0", Revision: "cafe", Time: "2026-01-01T00:00:00Z",
				Source: SourceBuildInfo,
			},
		},
		{
			name:      "local build: revision only, no version",
			ldVersion: "dev",
			info: bi(develVersion, map[string]string{
				"vcs.revision": "9ab1f88", "vcs.time": "2026-07-25T09:00:00Z",
			}),
			ok:   true,
			want: Info{Revision: "9ab1f88", Time: "2026-07-25T09:00:00Z", Source: SourceBuildInfo},
		},
		{
			name:      "local dirty build sets Modified",
			ldVersion: "dev",
			info: bi(develVersion, map[string]string{
				"vcs.revision": "9ab1f88", "vcs.modified": "true",
			}),
			ok:   true,
			want: Info{Revision: "9ab1f88", Modified: true, Source: SourceBuildInfo},
		},
		{
			name:      "no build info at all",
			ldVersion: "dev",
			info:      nil, ok: false,
			want: Info{Source: SourceUnknown},
		},
		{
			name:      "build info present but empty",
			ldVersion: "dev",
			info:      bi("", nil), ok: true,
			want: Info{Source: SourceUnknown},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve(tt.ldVersion, tt.ldCommit, tt.ldDate, tt.info, tt.ok)
			if got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDevelNeverSurfaces is the specific regression this feature exists to
// prevent: "(devel)" is the module system's placeholder, and reporting it to a
// user is the same defect as reporting "dev".
func TestDevelNeverSurfaces(t *testing.T) {
	cases := []*debug.BuildInfo{
		bi(develVersion, nil),
		bi(develVersion, map[string]string{"vcs.revision": "abc"}),
		bi(develVersion, map[string]string{"vcs.modified": "true"}),
	}
	for _, info := range cases {
		got := resolve("dev", "none", "unknown", info, true)
		if got.Version == develVersion {
			t.Errorf("resolve() surfaced %q as a version", develVersion)
		}
		if got.Version != "" {
			t.Errorf("resolve() invented version %q from a (devel) build", got.Version)
		}
	}
}

// TestDevSentinelNeverSurfaces guards the other placeholder: an unstamped
// binary must fall through to build metadata rather than reporting "dev".
func TestDevSentinelNeverSurfaces(t *testing.T) {
	got := resolve("dev", "none", "unknown", bi("v1.2.3", nil), true)
	if got.Version == devSentinel {
		t.Fatalf("resolve() surfaced the %q sentinel as a version", devSentinel)
	}
	if got.Version != "v1.2.3" {
		t.Errorf("Version = %q, want the module version to win over the sentinel", got.Version)
	}
	if got.Source != SourceBuildInfo {
		t.Errorf("Source = %v, want SourceBuildInfo", got.Source)
	}
}

func TestKnown(t *testing.T) {
	if (Info{}).Known() {
		t.Error("empty Info reported Known() = true")
	}
	if !(Info{Version: "v1.0.0"}).Known() {
		t.Error("Info with a version reported Known() = false")
	}
	// A revision-only build is deliberately not "known": the updater cannot
	// compare a revision against a release tag.
	if (Info{Revision: "abc"}).Known() {
		t.Error("revision-only Info reported Known() = true")
	}
}

func TestSourceString(t *testing.T) {
	for _, tt := range []struct {
		src  Source
		want string
	}{
		{SourceLDFlags, "release build"},
		{SourceBuildInfo, "build metadata"},
		{SourceUnknown, "unavailable"},
		{Source(99), "unavailable"},
	} {
		if got := tt.src.String(); got != tt.want {
			t.Errorf("Source(%d).String() = %q, want %q", tt.src, got, tt.want)
		}
	}
}

// TestResolveUsesRealBuildInfo exercises the exported wrapper against the
// actual test binary, so the debug.ReadBuildInfo call itself is covered.
func TestResolveUsesRealBuildInfo(t *testing.T) {
	got := Resolve("dev", "none", "unknown")
	if got.Version == devSentinel || got.Version == develVersion {
		t.Errorf("Resolve() surfaced placeholder %q", got.Version)
	}

	stampedInfo := Resolve("v1.2.3", "abc", "2026-01-01T00:00:00Z")
	if stampedInfo.Version != "v1.2.3" || stampedInfo.Source != SourceLDFlags {
		t.Errorf("Resolve() with stamped values = %+v, want ldflags to win", stampedInfo)
	}
}
