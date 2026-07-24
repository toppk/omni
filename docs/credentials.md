# Credentials

Omni reads local secrets from `~/.config/omni/credentials/credentials.toml`. Create the layout with:

```bash
omni configure init
```

The credential file is created with mode `0600`. Do not commit it, paste it into tickets, or place it in shell history. `settings.toml` is separate so ordinary policy preferences never need to share a file with secrets.

## Tailscale

The Tailscale integration is for Tailnet-wide administration; use the official `tailscale` program for controlling the local node.

1. Sign in to the Tailscale admin console for the Tailnet you intend to manage.
2. Create an API access token in the console's settings/keys area. Use the narrowest available permissions and an expiration appropriate to the task.
3. Copy it once and add it locally:

```toml
[tailscale]
api_key = "tskey-api-..."
```

4. Begin by using an `observe` command. Authorization, ACL application, and device deletion are separate effect-prefixed commands and should only use credentials permitted for those tasks.

If an organization uses OAuth clients or another managed credential mechanism, keep the client secret in this same local file and use the smallest provider scopes available. Omni will document the exact supported form before enabling that authentication path.

## Trello

Trello authenticates API requests with an API key and a user token.

1. While signed into Trello, follow the official [Trello API introduction and credential guide](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/) to create/copy an API key and user token. The same URL is compiled into Omni: `omni configure help trello.api-key`.
2. From that page, use the token-generation link. Review the requested access and select the least privilege and shortest lifetime that supports the intended work.
3. Store both values locally, without editing the secret file directly:

```bash
omni configure trello --api-key "your-api-key" --api-token "your-user-token"
```

4. Start with `omni observe trello board list` when provider calls are enabled. Use separate `create`, `move`, `archive`, and `delete` paths for changes; no observation flag will cause a mutation.

Revoke or rotate a token from Trello if it is exposed. Use an account with only the workspaces and boards needed for the operation.

## Current implementation status

The configuration layout, command registry, structured discovery, and runtime policy checks are implemented. Trello board listing and overview, board-list listing, card retrieval, card creation, card movement, card archival, and card deletion are live native API operations. Tailscale remains registered but is not implemented yet.
