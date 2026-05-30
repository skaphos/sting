# Registering OAuth Apps for Sting

> **Status**: Draft (as of 2026-05-30). This guide is being updated in parallel with the OAuth implementation (SKA-466).

Sting supports two authentication methods:

- **OAuth App flows** (recommended / primary path)
- **Personal Access Tokens (PATs)** (legacy fallback)

This guide explains how to register an OAuth App so Sting can use modern OAuth-based login (similar to `gh auth login` and `glab auth login --device`).

Sting ships with credentials for **official Skaphos-published OAuth Apps** on both github.com and gitlab.com. For self-hosted GitLab (or GitHub Enterprise Server) you register your own application. You can always register and use your own app on any provider.

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

Sting uses GitLab's native **Device Authorization Grant** flow (the same approach as `glab auth login --device`). This is the recommended path for CLIs.

Sting ships with credentials for an official Skaphos OAuth App on gitlab.com. The public app is registered as **non-confidential**, so only the Client ID is needed (`sting auth gitlab` works out of the box with no extra flags). For any self-hosted GitLab instance you must register your own application.

### Recommended Settings

| Field                              | Recommended Value                                      | Notes |
|------------------------------------|--------------------------------------------------------|-------|
| **Name**                           | `Sting CLI` (or `MyOrg Sting CLI`)                     | Clear and descriptive |
| **Redirect URI**                   | `http://127.0.0.1/callback` or `urn:ietf:wg:oauth:2.0:oob` | Required by the form; not used for pure device flow |
| **Scopes**                         | `read_api` (minimum and recommended)                   | Sting only reads commit history. `api` also works. |
| **Confidential**                   | **No** (recommended for the public gitlab.com app)     | Non-confidential apps do not have a client secret. Confidential apps require a secret. |
| **Device authorization grant flow**| **Enabled / Checked**                                  | **Critical** — this enables the flow Sting uses |

### Steps on GitLab.com

1. Go to **User Settings → Applications** (or for a group: the group → **Settings → Applications**).
2. Click **Add new application**.
3. Fill in the form using the table above.
4. **Important**: Check the box labeled **Device authorization grant flow**.
5. Select the `read_api` scope (or `api` if you prefer broader access).
6. Leave **Confidential** unchecked if you want the simplest experience (no client secret will be generated).
7. Click **Save application**.
8. Copy the **Application ID** — this is your Client ID.
9. (Only for confidential apps) Copy the **Secret** if you need it for `--client-secret`.

Then authenticate with:

```bash
sting auth gitlab --client-id <YOUR_APPLICATION_ID>
# or for a self-hosted instance:
sting auth gitlab --hostname gitlab.example.com --client-id <YOUR_ID>
```

### Steps on Self-Hosted GitLab

The process is identical, but you perform it on your own instance.

1. Log into your GitLab instance.
2. Go to your profile → **User Settings → Applications**, or the equivalent group/admin path.
3. Use the URL pattern: `https://gitlab.example.com/-/user_settings/applications`
4. Click **Add new application**.
5. Fill in the form (same table as above).
6. **Check "Device authorization grant flow"**.
7. Select `read_api` scope.
8. For the simplest setup, leave the app non-confidential (no secret generated).
9. Save and copy the **Application ID** (Client ID).

If the instance does not support device flow (older GitLab or the checkbox is missing), fall back to:

```bash
echo 'glpat-xxxxxxxxxxxx' | sting auth gitlab --hostname gitlab.example.com --with-token
```

Client secrets are only relevant when you deliberately create a confidential application. For normal use (including the public gitlab.com app) you only need the Client ID.

### Using Your Own App (Bring Your Own)

This is the normal path for GitLab today:

- Use `--client-id` (and optionally `--client-secret`) on the command line.
- Or set the environment variables `STING_GITLAB_CLIENT_ID` and `STING_GITLAB_CLIENT_SECRET`.
- These can be different per host when using `--hostname`.

This model is intentional and matches how most teams use `glab` with self-hosted GitLab. Organizations with strict requirements can register their own apps with custom names, scopes, and audit trails.

## Scopes Sting Needs

Sting only **reads** data. The following scopes are typically sufficient:

**GitHub**:
- `repo` (or more fine-grained equivalents)
- `read:org`
- `gist` (optional, for some future features)

**GitLab**:
- `read_api` (recommended — read-only access to the API, sufficient for commit history)
- `api` (broader, also works)

Sting requests the minimal scope needed (`read_api` for GitLab, `repo` + `read:org` for GitHub). You can start with the smallest scope and expand later. Sting will surface clear errors if required scopes are missing.

## Security Considerations

- CLI tools are **public clients**. The Client Secret must be embedded in the binary. This is an accepted pattern (see GitHub's official guidance on public clients and the `gh` CLI implementation).
- Prefer **Device Flow** when possible.
- For maximum security, many organizations choose to register their own OAuth App (or move to GitHub Apps in the future) rather than relying on a vendor-published app.

## Migration and Fallbacks

- Existing users with `token` / `STING_TOKEN` or `gitlab_token` continue to work unchanged.
- `sting auth status` will clearly indicate whether you are using OAuth tokens or legacy PATs.
- PATs are the documented fallback for situations where OAuth registration is not feasible.

## Next Steps / Status

- GitHub OAuth (with public Skaphos app + GHES bring-your-own) is fully implemented and documented.
- GitLab OAuth device flow is implemented with a public Skaphos app for gitlab.com + self-hosted bring-your-own support.
- This registration guide is being kept in sync with the code (see `sting auth gitlab --help` and the error messages for the latest instructions).

See also:
- [ADR 0007: OAuth App authentication and multi-provider credential storage](adr/0007-oauth-app-authentication.md)
- Main README (authentication section will be updated when the feature is more widely announced)

Feedback and improvements are welcome.

---

*Drafted on `feature/ska-466` following the Go engineering guidelines and decisions recorded in the ADR and Linear tickets.*