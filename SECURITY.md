# Security Policy

Sting reads GitHub and GitLab activity through a CLI and a stdio MCP server.
Its security-sensitive assets include provider credentials, private repository
content, the integrity of returned evidence, and local configuration written
by authentication and MCP installation commands.

## Reporting a vulnerability

**Please do not open a public issue or pull request for a suspected
vulnerability.** Use [GitHub private vulnerability reporting](https://github.com/skaphos/sting/security/advisories/new).
If that channel is unavailable or you cannot use GitHub, email
[shawn@skaphos.io](mailto:shawn@skaphos.io) with `sting` in the subject.

## Threat model and reportable issues

Sting runs with the invoking user's operating-system permissions and selected
provider identity. Repository content, commit messages, pull request text,
diffs, and provider responses can contain attacker-controlled data. They must
not become executable commands or authority to use a different identity.

Report suspected violations such as:

- Query commands or MCP activity tools modifying provider repositories, issues,
  or pull requests, including writes through a tool represented as read-only.
- Credentials leaking through output, logs, errors, redirects, or requests to
  an unintended provider or host.
- Authentication using a credential outside the user's configured selection,
  or bypassing consent required for insecure credential storage.
- Untrusted content causing code execution, unintended local file access,
  exploitable resource exhaustion, or disclosure outside the requested scope.
- Installation or credential operations overwriting unrelated configuration
  or writing outside their intended destinations.
- Attacker-controlled input corrupting the provenance or integrity of evidence
  used for security decisions. Explain the security impact; ordinary missing
  results or formatting errors may be correctness bugs.

Sting's provider queries are read-only; its authentication and installer
commands intentionally manage local state. The MCP server shares the user's
permissions and is not an isolation sandbox. These boundaries provide triage
context, not blanket exclusions: report privately when in doubt.

## What to include

Include the affected repository, version or commit, platform and relevant
configuration, reproduction steps or a minimal proof of concept, and the
impact you believe is possible. Explain what an attacker controls and which
security boundary is crossed. A suggested fix is welcome but not required.

Use synthetic data and redact credentials and private repository content.
Please report suspected issues even if you cannot reproduce them on the latest
release or are unsure whether they are vulnerabilities.

## Response and coordinated disclosure

- We aim to acknowledge reports within **7 days**. If you have not heard back,
  follow up by email with the repository name and the date of your report.
- Maintainers assess the report with the reporter, including affected versions,
  realistic exploitability, impact, and severity. A scanner alert alone does
  not establish impact, but reports do not need a complete exploit to be useful.
- We limit embargoed information to people needed for triage, remediation, and
  coordinated release. We develop and review security fixes privately and test
  that the reported issue is resolved before publishing the patch.
- We aim to release a fix or mitigation and disclose within **90 days** of the
  report. This is a coordination target, not a guaranteed fix date or an
  obligation on reporters to remain silent indefinitely. We discuss timing
  changes with the reporter; active exploitation can require earlier notice.
- We coordinate the patched release with a public security advisory describing
  impact, affected and fixed versions, and any mitigation. For vulnerabilities
  eligible for a CVE, we request an identifier through GitHub or another CVE
  Numbering Authority and include it in the advisory. Exploit details may be
  delayed to give users time to update.
- We credit reporters unless they prefer anonymity. If triage finds a regular
  bug or a hardening opportunity, we explain why and coordinate moving it to a
  public issue only after checking that doing so exposes no unresolved
  vulnerability.

## Versions and dependencies

Security fixes normally target the latest release. Report the version you use;
we assess affected versions and any need for backports during triage rather
than rejecting reports solely because they concern an older release. For
unreleased projects, include the commit you tested.

Dependency vulnerabilities are relevant when they affect this project's use of
the dependency. Include that context if known; maintainers will coordinate
with upstream as needed. Do not disclose an embargoed upstream issue publicly.

## Bug bounty

This policy does not offer a paid bug bounty.
