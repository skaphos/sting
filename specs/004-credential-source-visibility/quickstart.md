# Quickstart: Validating Credential Source Visibility

Every scenario runs against an isolated `HOME`, so your real credentials are never used. The
automated tests cover the same matrix without network access. This guide is for a manual
end-to-end check. The contract it validates is [credential-report.md](contracts/credential-report.md).

## Setup

```sh
go -C tools tool task build
export HOME="$(mktemp -d)" USERPROFILE="$HOME"
unset STING_TOKEN STING_GITLAB_TOKEN STING_ALLOW_AMBIENT_GITHUB_TOKEN STING_VERBOSE
```

## Scenarios

| # | Setup | Command | Expected stderr line |
|---|-------|---------|----------------------|
| 1 | none | `./sting query --author octocat --verbose -o json > out1.json` | `sting: credential: github github.com: anonymous` |
| 2 | `STING_TOKEN=<pat>` | same | `… github github.com: dedicated PAT` |
| 3 | `printf '<pat>\n' \| ./sting auth gitlab --with-token --insecure-storage --hostname gitlab.example.com` | `./sting query --provider gitlab --gitlab-base-url https://gitlab.example.com/api/v4 --scope repos --repos g/p --author someone --verbose` | `… gitlab gitlab.example.com: stored pat (file)` |
| 4 | none | `./sting query --provider gitlab --author someone --gitlab-base-url https://gitlab.example.com/api/v4 --verbose` | `… gitlab gitlab.example.com: anonymous` |
| 5 | `STING_VERBOSE=true` instead of the flag | scenario 1 command without `--verbose` | same as scenario 1 |
| 6 | scenario 2 setup, verbose **off** | `./sting query --author octocat -o json > out6.json 2> err6.txt` | `err6.txt` empty; `out6.json` has the same shape as `out1.json` |

The `activity`, `prs`, and `inbox` commands produce the same line under the same setups.

## MCP

```sh
STING_VERBOSE=true ./sting mcp 2> mcp-stderr.txt
```

Drive any of the four tools from an MCP client (or the `internal/mcpserver` tests). Each call
appends one `sting: credential: …` line to `mcp-stderr.txt`, and the protocol stream on stdout
is unchanged. Without `STING_VERBOSE`, `mcp-stderr.txt` stays empty.

## Secret check

Search every captured stderr file for the token value you used. There must be no match.
