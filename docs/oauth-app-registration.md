# Registering OAuth Apps for Sting

> **Status**: Draft (as of 2026-05-30). This guide will be finalized alongside the OAuth authentication feature (SKA-466).

Sting supports two authentication methods:

- **OAuth App flows** (recommended / primary path)
- **Personal Access Tokens (PATs)** (legacy fallback)

This guide explains how to register an OAuth App so Sting can use modern OAuth-based login (similar to `gh auth login` and `glab auth login --device`).

Sting will ship with credentials for an **official Skaphos-published OAuth App** for convenience on github.com and gitlab.com. You can (and often should) register and use your own app instead.

## Why Register an OAuth App?

- Better user experience (browser or device code flow, no manual token copying for most users).
- Proper support for GitHub Enterprise Server (GHES) and self-hosted GitLab.
- Token refresh capabilities (future).
- Clear separation of credentials from ambient `GITHUB_TOKEN` / `GITLAB_TOKEN`.

OAuth is the happy path. PATs remain fully supported for automation, CI, air-gapped environments, and cases where registering an app is impractical.

## GitHub (github.com and GitHub Enterprise Server)

### Recommended Settings

When registering the app, use these values:

| Field                        | Recommended Value                                      | Notes |
|-----------------------------|-------------------------------------------------------|-------|
| **Application name**        | `Sting CLI` (or `MyOrg Sting CLI`)                    | Clear and descriptive |
| **Homepage URL**            | `https://github.com/skaphos/sting` or your internal docs | Can be any valid URL |
| **Authorization callback URL** | `http://127.0.0.1/callback` (and optionally `http://localhost/`) | Required for web flow fallback. Match exactly what Sting will use. |
| **Device flow**             | **Enabled**                                           | Critical for good CLI experience |
| **Client secret**           | Generate one                                          | Will be embedded (public client reality for CLIs) |

### Steps on GitHub.com (public)

1. Go to **Settings → Developer settings → OAuth Apps** (or [https://github.com/settings/apps](https://github.com/settings/apps)).
2. Click **New OAuth App**.
3. Fill in the form using the table above.
4. Click **Register application**.
5. Copy the **Client ID**. Optionally generate a **Client Secret**.
6. (Optional but recommended) Note the app in your security/compliance records.

### Steps on GitHub Enterprise Server (GHES)

**Important**: OAuth Apps on GHES are registered **per GHES instance**, not on github.com.

1. Log into your GHES instance.
2. Go to your profile → **Settings → Developer settings → OAuth Apps**.
3. Click **New OAuth App**.
4. Use the same recommended settings above (the callback URLs are the same).
5. **Enable Device Flow**.
6. Save the **Client ID** and **Client Secret** for that specific GHES hostname.

You will need to provide these credentials to Sting when using `sting auth github --hostname your-ghes.example.com` (exact mechanism will be documented once the configuration surface is finalized).

### Using Your Own App with Sting

Sting will support configuring custom client credentials (via config file or environment variables) so teams can use their own registered apps. This is especially useful for:

- GHES instances
- Organizations with strict audit or branding requirements
- Fine-grained scope control

Until the configuration support lands, you can still use `--with-token` to supply a PAT as the fallback.

## GitLab (gitlab.com and self-hosted)

GitLab has excellent native support for the device authorization flow.

### Recommended Settings

| Field                    | Recommended Value                          | Notes |
|--------------------------|--------------------------------------------|-------|
| **Name**                 | `Sting CLI` (or `MyOrg Sting`)             | — |
| **Redirect URI**         | `http://127.0.0.1/callback`                | Used for web fallback |
| **Scopes**               | `api`, `read_repository`, `write_repository` (minimum) | Adjust based on your needs |
| **Confidential**         | **No** (public client)                     | Required for device flow in many setups |

### Steps on GitLab.com

1. Go to **User Settings → Applications** (or for group-level: group → Settings → Applications).
2. Click **Add new application**.
3. Fill in the form.
4. **Do not** check "Confidential".
5. Check the appropriate scopes.
6. Save and note the **Application ID** (Client ID) and **Secret**.

### Steps on Self-Hosted GitLab

The process is identical to GitLab.com but performed on your instance (`https://gitlab.example.com/-/user_settings/applications` or the equivalent admin/group path).

## Scopes Sting Needs

Sting only **reads** data. The following scopes are typically sufficient:

**GitHub**:
- `repo` (or more fine-grained equivalents)
- `read:org`
- `gist` (optional, for some future features)

**GitLab**:
- `api` (broad but convenient)
- Or the more granular `read_repository` + `read_user` etc.

You can start minimal and expand later. Sting will surface clear errors if required scopes are missing.

## Security Considerations

- CLI tools are **public clients**. The Client Secret must be embedded in the binary. This is an accepted pattern (see GitHub's official guidance on public clients and the `gh` CLI implementation).
- Prefer **Device Flow** when possible.
- For maximum security, many organizations choose to register their own OAuth App (or move to GitHub Apps in the future) rather than relying on a vendor-published app.

## Migration and Fallbacks

- Existing users with `token` / `STING_TOKEN` or `gitlab_token` continue to work unchanged.
- `sting auth status` will clearly indicate whether you are using OAuth tokens or legacy PATs.
- PATs are the documented fallback for situations where OAuth registration is not feasible.

## Next Steps / Status

This guide is being developed in parallel with the OAuth implementation (Linear SKA-466 / SKA-467).

See also:
- [ADR 0007: OAuth App authentication and multi-provider credential storage](adr/0007-oauth-app-authentication.md)
- Main README (authentication section will be updated)

Feedback and improvements to this draft are welcome.

---

*Drafted on `feature/ska-466` following the Go engineering guidelines and decisions recorded in the ADR and Linear tickets.*