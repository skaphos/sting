// SPDX-License-Identifier: MIT

package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Owner identifies what put the running binary where it is. It decides whether
// sting may replace itself and, when it may not, which upgrade instruction is
// the correct one to print.
type Owner int

const (
	// OwnerUnmanaged means the binary was placed by hand. This is the only
	// case in which sting replaces itself.
	OwnerUnmanaged Owner = iota
	OwnerHomebrew
	OwnerRPM
	OwnerDPKG
	OwnerGoToolchain
	// OwnerUndeterminable means the executable path could not be resolved.
	// sting refuses to write rather than guessing a path.
	OwnerUndeterminable
)

// String renders the owner for human-readable output.
func (o Owner) String() string {
	switch o {
	case OwnerHomebrew:
		return "Homebrew"
	case OwnerRPM:
		return "an RPM package"
	case OwnerDPKG:
		return "a Debian package"
	case OwnerGoToolchain:
		return "the Go toolchain"
	case OwnerUndeterminable:
		return "an undeterminable location"
	default:
		return "unmanaged"
	}
}

// Managed reports whether another installer owns this file. Overwriting a
// package-managed file breaks that manager's own verification and gets clobbered
// by its next upgrade, so sting refuses.
func (o Owner) Managed() bool {
	return o != OwnerUnmanaged
}

// Provenance is the resolved answer to "who owns the running binary".
type Provenance struct {
	Owner          Owner
	RealPath       string
	UpgradeCommand string
}

// commandRunner reports whether a command succeeded. It is a seam so ownership
// detection is testable without rpm or dpkg installed.
type commandRunner func(name string, args ...string) error

func runCommand(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// classify determines install provenance. Order matters: the checks overlap,
// and a Homebrew binary on PATH is a symlink into a versioned cellar, so the
// path must be fully resolved before anything is judged.
func classify(execPath func() (string, error), run commandRunner) Provenance {
	raw, err := execPath()
	if err != nil {
		return Provenance{Owner: OwnerUndeterminable}
	}

	real, err := filepath.EvalSymlinks(raw)
	if err != nil {
		// The path exists enough to have been executed but cannot be
		// resolved. Refusing is the safe answer; writing to an
		// unresolved path risks clobbering a link target.
		return Provenance{Owner: OwnerUndeterminable, RealPath: raw}
	}

	switch {
	case isHomebrew(real):
		return Provenance{
			Owner:          OwnerHomebrew,
			RealPath:       real,
			UpgradeCommand: "brew upgrade --cask sting",
		}
	case ownedBy(run, "rpm", "-qf", real):
		return Provenance{
			Owner:          OwnerRPM,
			RealPath:       real,
			UpgradeCommand: "sudo dnf upgrade sting",
		}
	case ownedBy(run, "dpkg", "-S", real):
		return Provenance{
			Owner:    OwnerDPKG,
			RealPath: real,
			// Deliberately not "apt upgrade": there is no hosted apt
			// repository, so the honest instruction is to fetch the
			// next release's package.
			UpgradeCommand: "download the latest .deb from " + releasesPage + " and run: sudo dpkg -i sting_<version>_<arch>.deb",
		}
	case isGoToolchain(real):
		return Provenance{
			Owner:          OwnerGoToolchain,
			RealPath:       real,
			UpgradeCommand: "go install github.com/skaphos/sting/cmd/sting@latest",
		}
	default:
		return Provenance{Owner: OwnerUnmanaged, RealPath: real}
	}
}

// isHomebrew tests the path rather than invoking brew, so detection still works
// when brew is not on PATH.
func isHomebrew(path string) bool {
	if prefix := os.Getenv("HOMEBREW_PREFIX"); prefix != "" && underDir(path, prefix) {
		return true
	}
	// Cellar and Caskroom are the versioned stores everything on PATH links
	// into; matching them catches an install under any prefix.
	for _, marker := range []string{"/Cellar/", "/Caskroom/"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	for _, prefix := range []string{"/opt/homebrew", "/home/linuxbrew/.linuxbrew"} {
		if underDir(path, prefix) {
			return true
		}
	}
	return false
}

// isGoToolchain covers the locations `go install` writes to.
func isGoToolchain(path string) bool {
	if gobin := os.Getenv("GOBIN"); gobin != "" && underDir(path, gobin) {
		return true
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		for _, p := range filepath.SplitList(gopath) {
			if underDir(path, filepath.Join(p, "bin")) {
				return true
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return underDir(path, filepath.Join(home, "go", "bin"))
	}
	return false
}

// ownedBy reports whether a package manager claims this file. A missing manager
// is not an error: rpm and dpkg only exist where their databases do, so absence
// means "not owned by this one".
func ownedBy(run commandRunner, name string, args ...string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	return run(name, args...) == nil
}

// underDir reports whether path sits inside dir, comparing whole path segments
// so that "/opt/homebrew-other" does not match "/opt/homebrew".
func underDir(path, dir string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(dir), path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
