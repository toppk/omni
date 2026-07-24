# Tailscale

Omni uses the [Tailscale API](https://tailscale.com/api) for tailnet-wide
administration. It does not replace the local `tailscale` client for managing
the computer on which Omni runs.

## API reference

Tailscale's [interactive API reference](https://tailscale.com/api) is the
provider source for endpoint requests and response schemas. The companion
[API authentication reference](https://tailscale.com/docs/reference/tailscale-api)
and [trust credentials reference](https://tailscale.com/docs/reference/trust-credentials)
define authentication and OAuth scopes. Record each endpoint Omni adopts in
this document and in the command registry; do not infer unsupported endpoints
from an undocumented schema.

The checked-in [OpenAPI v2 snapshot](tailscale-openapi-v2-snapshot-20260724.json)
captures the provider contract used for this service work on 2026-07-24. It is
an investigation input, not a promise of permanent provider compatibility:
Tailscale marks the source OpenAPI specification as unstable even while its
documented endpoints are stable.

## Initial surface

The first implementation intentionally stays narrow:

- list and inspect devices, including filtering an identity tag such as an
  ephemeral CI runner;
- list a device's advertised routes;
- rename a device and replace its tag set;
- download, validate, preview, and apply the tailnet ACL file;
- list and inspect users.

The API endpoints and required scopes are recorded in Tailscale's [trust
credentials reference](https://tailscale.com/docs/reference/trust-credentials).
A single `all` token is not required and should not be the default choice.
When a supported command receives a 403, Omni names the relevant scope in its
error.

### Fine-grained OAuth scopes

Grant only the rows needed by the commands a client will use. Tailscale has
used both `acl` and `policy_file` names for the ACL scope family; use the name
shown in your Trust credentials UI.

| Omni command area | Read scope | Write scope, only when needed |
| --- | --- | --- |
| devices: list and get | `devices:core:read` | `devices:core` for name or tag changes |
| device routes | `devices:routes:read` | none in Omni's current surface |
| users | `users:read` | none in Omni's current surface |
| ACL get, validate, preview | `acl:read` or `policy_file:read` | `acl` or `policy_file` for `administer tailscale acl set` |
| DNS snapshot | `dns:read` | none in Omni's current surface |
| auth-key metadata | `auth_keys:read` | none in Omni's current surface |
| API-token metadata | `api_access_tokens:read` | none in Omni's current surface |
| OAuth-client metadata | `oauth_keys:read` | none in Omni's current surface |

Tailscale also requires `devices:core:read` and
`devices:posture_attributes:read` alongside the ACL read scope. The
`observe tailscale key list` command is intentionally non-secret: with
`auth_keys:read`, it returns auth-key metadata visible to that credential; with
`api_access_tokens:read`, it returns visible API-token metadata; and with
`oauth_keys:read`, it returns OAuth-client and OAuth-token metadata. It does
not reveal any key value. Omni actively uses `api_access_tokens:read` for
`observe tailscale key list` when auditing API-token expiry and scope, and
`oauth_keys:read` when auditing OAuth-client scopes and tags.

Device lists default to only `id`, `hostname`, `os`, and `lastSeen`. Add
`--details` to include addresses, client version, tags, ownership, and related
selection context without issuing a separate `get` request for every device.

## Setup

Choose one authentication method. Both ultimately send an access token to the
Tailscale API, but they obtain it differently.

| Method | Use it when | Stored by Omni |
| --- | --- | --- |
| API access token | You want the simplest individual setup. It has broad tailnet access and expires after the chosen 1–90 days. | `tailscale.api-key` secret |
| OAuth client | You need delegated, fine-grained scopes for automation. Omni exchanges its client ID and secret for a short-lived token, then reuses it until shortly before expiry. | `tailscale.client-id` setting and `tailscale.client-secret` secret |

Tailscale documents the simple API-token path in its [API authentication
reference](https://tailscale.com/docs/reference/tailscale-api), and the scoped
path in its [OAuth client guide](https://tailscale.com/docs/features/oauth-clients).
If both are configured, Omni uses the explicit API access token.

The tailnet defaults to `-`, Tailscale's shorthand for the tailnet owning the
access token. Override it only when the credential is entitled to another
tailnet.

```bash
omni setup tailscale

# Simple API token
omni configure tailscale --api-key ACCESS_TOKEN

# Scoped OAuth client
omni configure tailscale --client-id CLIENT_ID --client-secret CLIENT_SECRET
# optional explicit target or compatible mock server
omni configure tailscale --tailnet TAILNET_ID --api-url URL
```

The client ID is an ordinary setting. The client secret and API token are stored
only in `~/.config/omni/credentials/credentials.toml`. An OAuth-generated token
is cached with its expiry in `~/.local/share/omni/ephemeral/credentials.toml`
(or `$XDG_DATA_HOME/omni/ephemeral/credentials.toml`), with directory mode
`0700` and file mode `0600`. Omni renews it five minutes before expiry.
`tailscale.api-url` is a registry default, so a compatible mock API can be
selected without changing source.

Remove a value without manually editing TOML:

```bash
omni configure delete tailscale.client-id
omni configure secret delete tailscale.client-secret
omni configure secret delete tailscale.api-key
omni configure secret delete tailscale.generated-api-key
```

## ACL files

Tailscale ACL configuration is HuJSON. The [policy syntax
reference](https://tailscale.com/docs/reference/syntax/policy-file) recommends
grants for new access-control policy, while retaining ACL compatibility.

`observe tailscale acl get` always writes the ACL to a new local file,
instead of emitting the full policy to stdout. Use `--output PATH` to choose the
file name. It never overwrites an existing file and creates the file as `0600`.

Validate a local candidate without changing the tailnet:

```bash
omni observe tailscale acl validate ACL.hujson
```

This calls Tailscale's validation endpoint, including any ACL tests in the
file. It is an `observe` operation; applying an ACL remains separate. A failed
validation prints `status: invalid` and exits nonzero, so it is safe to use as
the gate before an administration command.

Preview access for an identity against the active ACL, or a candidate file:

```bash
omni observe tailscale acl preview --for tag:github
omni observe tailscale acl preview --for tag:github --file ACL.hujson
```

The preview is evaluated by Tailscale rather than a local HuJSON approximation;
it returns the matching rules without applying the candidate file.

## Keys and DNS

Read-only key inspection reports accessible key IDs and metadata such as key
type, OAuth scopes, trust-credential tags, creator, invalid/revoked state, and
auth-key ephemeral/reusable/preauthorized properties when the API returns them.
It never emits a key secret:

```bash
omni observe tailscale key list
omni observe tailscale key list --all
omni observe tailscale key get KEY_ID
```

The default lists active keys for the current credential type. `--all` asks for
all visible active key types and may require additional read scopes. Tailscale
does not enumerate revoked or invalid keys in list results; use `key get` with
a known key ID to inspect its `invalid` or `revoked` state. OAuth-client
records are useful for auditing the scopes and tags that constrain a GitHub
Action's ephemeral identity; the workflow's configured `tags:` value remains
the identity it requests.

Inspect DNS as one compact snapshot when assessing a host-based ACL:

```bash
omni observe tailscale dns get
```

It reports MagicDNS preferences, nameservers, search paths, and split-DNS.

To audit currently present devices with a CI identity without polling each
device, use:

```bash
omni observe tailscale device list --tag tag:github --details
```

## Ephemeral GitHub runners

The Tailscale GitHub Action creates a tagged ephemeral node for each workflow
run. Access is determined by that tag, not by the GitHub workflow itself. Keep
the runner tag narrowly scoped in the policy, then use the two observation
commands above to download a reviewable snapshot and validate a proposed change
before any `administer` command. Tailscale recommends a reusable, ephemeral,
tagged identity for this integration.

Applying an ACL is deliberately a distinct high-impact operation:

```bash
administer tailscale acl set ACL.hujson --backup acl-before-change.hujson
```

It validates the candidate itself, writes the active ACL to the required new
`0600` backup file, and sends the replacement with the ETag from that snapshot.
The ETag is Tailscale's optimistic-concurrency guard: it ties the replacement
to the exact ACL version that Omni backed up. If another actor changes the ACL
between snapshot and replacement, Tailscale rejects the stale `If-Match` write
with HTTP `412 Precondition Failed` and Omni leaves the active ACL unchanged.
This prevents one administrator or automation run from silently clobbering
another's change. Restore a snapshot by supplying it as `ACL.hujson` in a later
guarded set operation.
If the guarded replacement fails after the snapshot was written, retain that
backup for review and choose a new `--backup` path for a later retry: the live
ACL may no longer match the earlier snapshot.
