# ADR 0013: Homebrew cask relies on notarization, not a quarantine hook

**Status:** Accepted
**Date:** 2026-10-11
**Author:** Shawn Stratton

## Context

sting's Homebrew cask carried a `postflight` hook, copied from repokeeper ADR-0007, that ran
`xattr -dr com.apple.quarantine` on the staged binary. The hook existed because unsigned binaries
installed through a cask are refused by Gatekeeper.

The darwin binaries are now Developer ID signed (hardened runtime, secure timestamp) and notarized
through quill. For the `v1.2.1` arm64 binary, `spctl -a -t install` reports
`source=Notarized Developer ID`.

Homebrew now deprecates block-form `postflight` in favor of `postflight_steps`. Every `brew`
command that loads the `skaphos/tools` tap prints a deprecation warning. The tap's CI also fails
`brew style`: `Cask/InstallSteps` rejects the stanza, and because GoReleaser writes the custom
block first, `Cask/StanzaOrder` flags every other stanza too.

## Decision

Remove the quarantine hook from the cask. Do not port it to `postflight_steps`, because that
would keep a Gatekeeper bypass the binary no longer needs.

Because nothing in the cask covers for a missing signature anymore, the release workflow now
fails before GoReleaser runs if any `MACOS_*` secret is unset. Previously GoReleaser skipped
notarization without failing the release. This follows the same rule as the Homebrew tap
credential check: a release that cannot publish a working channel fails.

repokeeper records the same decision in its ADR-0020.

## Consequences

- The rendered cask has no `postflight` block. The deprecation warning and the tap CI failures
  for `Casks/sting.rb` stop once the next release overwrites it.
- Notarization tickets cannot be stapled to a bare Mach-O in a tarball. Gatekeeper checks the
  ticket online on first run, so a first run with no network access straight after install may be
  refused.
- The Developer ID certificate's expiry date is now a release dependency. Releases stop if signing
  fails, and they can no longer fall back to unsigned binaries. Rotate `MACOS_SIGN_P12` /
  `MACOS_SIGN_PASSWORD` before it expires. Binaries signed earlier stay valid because their
  signatures carry a secure timestamp.
