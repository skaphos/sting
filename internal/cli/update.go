// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/skaphos/sting/internal/buildinfo"
	"github.com/skaphos/sting/internal/selfupdate"
	"github.com/spf13/cobra"
)

// Exit statuses. One code per outcome class, so the command can be scripted
// against without parsing prose.
const (
	exitVerificationFailed = 1
	exitPackageManaged     = 2
	exitPlatformGated      = 3
	exitRefused            = 4
	exitPermission         = 5
)

var (
	updateCheckOnly bool
	updateTarget    string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update sting to the latest release",
	Long: "Downloads the latest release, verifies it was published by sting's own release\n" +
		"workflow, and replaces the running binary.\n\n" +
		"If another installer owns the binary -- Homebrew, an OS package, or the Go\n" +
		"toolchain -- sting prints that channel's upgrade command and changes nothing.\n\n" +
		"Verification is mandatory: the release's signature and checksums are always\n" +
		"checked before anything is written, and there is no flag to skip it.\n\n" +
		"This is the only command that contacts the network to check for a new version.",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false,
		"report what would happen without downloading or writing anything")
	updateCmd.Flags().StringVar(&updateTarget, "version", "",
		"update to this exact release tag instead of the latest (also allows rolling back)")
}

// newUpdater builds the updater. It is a package-level variable so tests can
// substitute one wired to a local server rather than the real release API.
var newUpdater = selfupdate.New

func runUpdate(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	updater := newUpdater()

	plan, err := updater.Plan(cmd.Context(), resolved(), updateTarget)
	if err != nil {
		// Could not resolve a release at all: not found, still
		// publishing, or the unauthenticated quota is exhausted. None
		// of these is a verification problem.
		return &exitError{code: exitRefused, message: err.Error()}
	}

	if updateCheckOnly {
		writeCheck(out, plan)
		return nil
	}

	switch plan.Action {
	case selfupdate.ActionUpToDate:
		fmt.Fprintf(out, "sting %s is already the latest release.\n", plan.Target)
		return nil

	case selfupdate.ActionDeferToManager:
		writeDeferral(out, plan)
		return &exitError{code: exitCodeFor(plan.Action)}

	case selfupdate.ActionGated:
		writeGate(out, plan)
		return &exitError{code: exitCodeFor(plan.Action)}

	case selfupdate.ActionRefuse:
		fmt.Fprintf(out, "Update refused: %s\n", plan.Reason)
		return &exitError{code: exitCodeFor(plan.Action)}

	case selfupdate.ActionReplace:
		return applyUpdate(cmd.Context(), out, updater, plan)
	}

	return &exitError{code: exitRefused, message: "unrecognised update action"}
}

// exitCodeFor maps an outcome to its documented exit status. One code per
// outcome class, so callers can branch on the status without parsing output.
func exitCodeFor(action selfupdate.Action) int {
	switch action {
	case selfupdate.ActionUpToDate, selfupdate.ActionReplace:
		return 0
	case selfupdate.ActionDeferToManager:
		return exitPackageManaged
	case selfupdate.ActionGated:
		return exitPlatformGated
	default:
		return exitRefused
	}
}

func applyUpdate(ctx context.Context, out io.Writer, updater *selfupdate.Updater, plan *selfupdate.Plan) error {
	if err := updater.Apply(ctx, plan); err != nil {
		switch {
		case errors.Is(err, selfupdate.ErrVerification):
			fmt.Fprintf(out, "Update refused: %s\nThe installed binary was not modified.\n", err)
			return &exitError{code: exitVerificationFailed}
		case errors.Is(err, selfupdate.ErrPermission):
			fmt.Fprintf(out, "Update failed: %s\n", err)
			return &exitError{code: exitPermission}
		default:
			return &exitError{code: exitRefused, message: err.Error()}
		}
	}

	fmt.Fprintf(out, "sting %s -> %s\n", displayVersion(plan.Current), plan.Target)
	fmt.Fprintln(out, "  verified: publisher signature and checksum OK")
	fmt.Fprintf(out, "  replaced: %s\n", plan.Provenance.RealPath)
	return nil
}

func writeCheck(out io.Writer, plan *selfupdate.Plan) {
	fmt.Fprintf(out, "  current:   %s (source: %s)\n", displayVersion(plan.Current), plan.Current.Source)
	if plan.Target != "" {
		fmt.Fprintf(out, "  latest:    %s\n", plan.Target)
	}
	if plan.Provenance.RealPath != "" {
		fmt.Fprintf(out, "  install:   %s (%s)\n", plan.Provenance.RealPath, plan.Provenance.Owner)
	}
	fmt.Fprintf(out, "  action:    %s\n", checkAction(plan))
}

// checkAction describes, in the conditional, what a real run would do.
func checkAction(plan *selfupdate.Plan) string {
	switch plan.Action {
	case selfupdate.ActionReplace:
		return "would replace"
	case selfupdate.ActionUpToDate:
		return "already up to date"
	case selfupdate.ActionDeferToManager:
		return "would defer to " + plan.Provenance.Owner.String()
	case selfupdate.ActionGated:
		return "would refuse: " + plan.Reason
	default:
		return "would refuse: " + plan.Reason
	}
}

func writeDeferral(out io.Writer, plan *selfupdate.Plan) {
	fmt.Fprintf(out, "%s:\n\n    %s\n\n", plan.Reason, plan.Provenance.UpgradeCommand)
	fmt.Fprintln(out, "Refusing to overwrite a package-managed file.")
}

func writeGate(out io.Writer, plan *selfupdate.Plan) {
	fmt.Fprintf(out, "sting %s is available.\n\n", plan.Target)
	fmt.Fprintf(out, "%s, and sting will not ask you to self-install an unsigned\nreplacement.\n\n", plan.Reason)
	fmt.Fprintf(out, "Download: https://github.com/skaphos/sting/releases/tag/%s\n", plan.Target)
}

// displayVersion renders a version for human output, saying so plainly when
// none was recorded rather than printing an empty string.
func displayVersion(info buildinfo.Info) string {
	if info.Known() {
		return info.Version
	}
	return "(version unavailable)"
}
