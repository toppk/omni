# Credentials

Omni reads local secrets from `~/.config/omni/credentials/credentials.toml`. Create the layout with:

```bash
omni configure init
```

The credential file is created with mode `0600`. Do not commit it, paste it into tickets, or place it in shell history. `settings.toml` is separate so ordinary policy preferences never need to share a file with secrets. Generated short-lived provider tokens, when supported, use a separate secured XDG-data store rather than this user-managed file.

## Tailscale

The Tailscale integration is for Tailnet-wide administration; use the official `tailscale` program for controlling the local node.

1. Sign in to the Tailscale admin console for the tailnet you intend to manage.
2. Choose either a manually generated API access token or a scoped OAuth client. API tokens are the simple broad-access option; [trust credentials](https://tailscale.com/docs/reference/trust-credentials) provide delegated fine-grained OAuth access.
3. Configure one path locally:

```bash
# Simple API token
omni configure tailscale --api-key ACCESS_TOKEN

# Scoped OAuth client
omni configure tailscale --client-id CLIENT_ID --client-secret CLIENT_SECRET
```

The OAuth client ID is stored in `settings.toml`; the OAuth client secret and
API token are stored in the credential file. Omni caches an OAuth access token
and expiry under `~/.local/share/omni/ephemeral/`, separately from user-managed
credentials, and renews it five minutes before expiry.

If both are present, Omni deliberately uses the explicit API token. Remove one
without manual file editing:

```bash
omni configure delete tailscale.client-id
omni configure secret delete tailscale.client-secret
omni configure secret delete tailscale.api-key
```

4. Begin by using an `observe` command. Tag replacement and policy application are separate effect-prefixed commands and should only use credentials permitted for those tasks.

Use the smallest OAuth scopes available.

## Trello

Trello authenticates API requests with an API key and a user token.

1. Run `omni configure trello` to view the local configuration guide, then, while signed into Trello, read the official [API introduction](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/), follow the [app-management walkthrough](https://developer.atlassian.com/cloud/trello/guides/power-ups/managing-apps/), and create or manage the API key at [trello.com/apps/admin](https://trello.com/apps/admin). `omni setup trello` directs you to this guide.
2. From that page, use the token-generation link. Review the requested access and select the least privilege and shortest lifetime that supports the intended work.
3. Store both values locally, without editing the secret file directly:

```bash
omni configure trello --api-key "your-api-key" --api-token "your-user-token"
```

4. Start with `omni observe trello board list` when provider calls are enabled. Use separate `create`, `move`, `archive`, and `delete` paths for changes; no observation flag will cause a mutation.

Revoke or rotate a token from Trello if it is exposed. Use an account with only the workspaces and boards needed for the operation.

For Omni's current agent-oriented setup, the Power-Up exists only to produce
the API key. On that Power-Up's API Key tab, click **Generate a Token**, approve
access, and copy the resulting user token; do not provide the app secret to
Omni. Trello also documents an application-mediated OAuth flow, but Omni does
not currently use it.

## Current implementation status

The configuration layout, command registry, structured discovery, and runtime policy checks are implemented. Trello board listing and overview, board-list listing, card retrieval, card creation, card movement, card archival, and card deletion are live native API operations. Tailscale device, route, policy, and user operations are also implemented as native API operations.
