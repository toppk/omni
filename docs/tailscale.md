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

The integration intentionally keeps host work and control-plane work separate:

- list and inspect devices, including filtering an identity tag such as an
  ephemeral CI runner;
- list a device's advertised routes, rename it, replace its tag set, approve
  enrollment, control or immediately trigger key expiry, and remove it;
- create machine auth keys for enrollment and revoke keys or trust credentials;
- download, validate, preview, and apply the tailnet ACL file;
- list and inspect users.

Omni does not install the Tailscale client or run `tailscale up` on a host.
Ansible (or another host provisioner) remains responsible for those steps and
can consume a short-lived auth key created by Omni.

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
| devices: list and get | `devices:core:read` | `devices:core` for name, tag, authorization, expiry, or removal changes |
| device routes | `devices:routes:read` | none in Omni's current surface |
| users | `users:read` | none in Omni's current surface |
| ACL get, validate, preview | `acl:read` or `policy_file:read` | `acl` or `policy_file` for `administer tailscale acl set` |
| DNS snapshot | `dns:read` | none in Omni's current surface |
| auth keys | `auth_keys:read` | `auth_keys` to create or revoke |
| API-token metadata | `api_access_tokens:read` | `api_access_tokens` to revoke |
| OAuth-client metadata | `oauth_keys:read` | `oauth_keys` to revoke |

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

## Enrollment, device lifecycle, keys, and DNS

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

Create a machine auth key for Ansible enrollment. The output path must not
exist; Omni reserves it before making the remote request and writes the
one-time secret with mode `0600`. The secret is never included in command
output:

```bash
omni create tailscale key auth create \
  --output ./server-enrollment.key \
  --description "server enrollment" \
  --expiry-seconds 3600 \
  --tag tag:server \
  --preauthorized
```

Omit `--preauthorized` when enrollment must remain pending until a separately
approved authorization operation. `--reusable` allows more than one device to
use the key, and `--ephemeral` requests automatic cleanup for short-lived
nodes. Revoke the credential as soon as the enrollment window closes:

```bash
omni delete tailscale key revoke KEY_ID
```

Ansible should read the private file, install Tailscale on the host, pass its
contents to `tailscale up --auth-key=...` in a task marked `no_log: true`, and
remove its temporary copy. The controller copy should also be deleted after a
successful run. Key revocation prevents later enrollment but does not remove
devices that already joined.

For tailnets requiring device approval, approve or deauthorize an enrolled
device explicitly:

```bash
omni authorize tailscale device authorization set DEVICE_ID --state authorized
omni authorize tailscale device authorization set DEVICE_ID --state unauthorized
```

Scheduled device-key expiry can be toggled, or the key can be expired
immediately. Immediate expiry cuts connectivity and requires host-side
re-authentication:

```bash
omni update tailscale device key expiry set DEVICE_ID --state disabled
omni update tailscale device key expiry set DEVICE_ID --state enabled
omni administer tailscale device key expire DEVICE_ID
```

Permanent removal is a distinct delete operation. Re-enrollment requires a new
host-side `tailscale up`:

```bash
omni delete tailscale device delete DEVICE_ID
```

All of these mutations are non-unattended operations in Omni's command
contract. `read-only` and `unattended-safe` policy modes reject them, leaving
`omni observe tailscale ...` suitable for Runbook's initial reporting-only
integration and allowing approval gates to be added around the explicit
mutation effects later.

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
