# Omni

[![Latest release](https://img.shields.io/github/v/release/toppk/omni?display_name=tag&sort=semver)](https://github.com/toppk/omni/releases/latest)
[![Release build](https://github.com/toppk/omni/actions/workflows/release.yml/badge.svg)](https://github.com/toppk/omni/actions/workflows/release.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/toppk/omni)](go.mod)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-42e8ff.svg)](LICENSE)

Omni is a dependency-light Go CLI that exposes service APIs through command paths whose safety class is visible at the left edge of the command.

## Active services

Omni currently ships **two live native service integrations**—Trello and
Tailscale. Both have setup and credential guidance, action-first discovery,
local policy enforcement, and tested API request paths.

| Service | What Omni can do today |
| --- | --- |
| [Trello](docs/trello.md) | Observe boards, lists, and cards; create, move, archive, and delete cards. |
| [Tailscale](docs/tailscale.md) | Observe devices, routes, users, and policy; deliberately update names, tags, and policy files. |

```bash
omni describe trello
omni describe tailscale
```

```text
omni observe trello board list
omni observe trello card get CARD_ID
omni create trello card create LIST_ID --name "Plan release"
omni delete trello card delete CARD_ID
```

The first word after `omni` is always the effect level. A flag must never turn an `observe` command into a mutating operation. This makes approval rules and local policy reliable without parsing arbitrary arguments.

## Start here

```bash
go test ./...
go run ./cmd/omni configure init
go run ./cmd/omni describe --format=json
```

Check the installed release version with `omni version`, `omni --version`, or `omni -V`. Local builds report `dev`; release builds receive their version from the Git tag during compilation.

`configure init` creates Omni's user-managed configuration files:

```text
~/.config/omni/settings.toml
~/.config/omni/credentials/credentials.toml
```

Populate Trello in one command, rather than editing files manually:

```bash
omni setup trello
omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN

# Inspect every registry key, its type, and whether it has a code default.
omni configure describe
omni configure trello
```

The Trello command stores ordinary settings in `settings.toml` and secrets only in `credentials/credentials.toml`. Commands acknowledge keys but never print secret values. Shell arguments can remain in history or be visible to local processes, so use a short-lived shell or clear the relevant history entry after using the convenience form.

`omni configure trello` prints the local setup and configuration guide without creating configuration files. Omni's current agent-oriented setup is intentionally manual: create the minimal Power-Up necessary for the API key, then use the API Key tab's Token link to generate a user token. The Power-Up app secret is not needed. Advanced registry setters and removal commands are available through `omni configure set`, `omni configure delete`, `omni configure secret set`, and `omni configure secret delete`.

For example, `trello.api-url` is an optional setting that defaults to `https://api.trello.com/1`. Point it at a compatible mock server with `omni configure trello --api-url URL`; this override applies to the native Trello client without changing code.

See [provider documentation](docs/index.md) and [credential setup](docs/credentials.md) before adding API credentials.

## Install

Linux (x86_64 and ARM64) and macOS (Intel and Apple Silicon) releases are distributed as single binaries with SHA-256 checksums. Install the latest version without elevated permissions:

```bash
curl -fsSL https://toppk.github.io/omni/install.sh | sh
```

The script detects the supported operating system and architecture, downloads the matching asset, verifies its release checksum, and installs it to `~/.local/bin`. It reports whether it installed, upgraded, or found the same version already installed; it never replaces an identical version. It warns if `~/.local/bin` is not on `PATH`, but does not change shell configuration. It can replace an Omni release or a local Omni development build at the target path, while refusing to overwrite an unrelated executable or conflict with another `omni` command on `PATH`. Set `OMNI_INSTALL_DIR` to choose another destination. Read the script before piping it to your shell, or download the binary and checksum directly from [GitHub Releases](https://github.com/toppk/omni/releases/latest).

## Output

Service commands render compact text for people by default. Request the
canonical machine-readable response with `--format json`; the presentation flag
may appear anywhere after the effect, though examples keep it at the end:

```bash
omni observe trello board list
omni observe trello board list --format json
OMNI_OUTPUT=json omni observe trello board list
```

`OMNI_OUTPUT` accepts `text` or `json` and sets the default for a process. An
explicit `--format text|json` overrides it. JSON remains the stable contract
for agents and scripts; text is rendered from the same compact response. Text
card collections are deliberately scan-oriented (name, checklist progress,
labels, member initials, due date, and list), while JSON retains the complete
compact card record.

## Trello commands

After adding a Trello API key and token, commands return compact records for
people, shell tools, and agents:

```bash
omni observe trello board list
omni observe trello board overview BOARD_ID
omni observe trello list list BOARD_ID
omni observe trello card get CARD_ID

omni create trello card create LIST_ID --name "Plan release" --description "Initial draft"
omni move trello card move CARD_ID LIST_ID --position bottom
omni archive trello card archive CARD_ID
omni delete trello card delete CARD_ID
```

The first four are `observe` commands. The change operations have their own action-level prefix, so no flag can transform an approved observation into a modification.

## Tailscale commands

Tailscale can use either a simple broad API access token or a scoped OAuth
client. The full credential and token-cache behavior is in
[docs/tailscale.md](docs/tailscale.md). It defaults to the tailnet that owns
the active token:

```bash
omni setup tailscale
omni configure tailscale --api-key ACCESS_TOKEN
# Or use a scoped OAuth client:
omni configure tailscale --client-id CLIENT_ID --client-secret CLIENT_SECRET

omni observe tailscale device list
omni observe tailscale device list --details
omni observe tailscale device get DEVICE_ID
omni observe tailscale device route list DEVICE_ID
omni update tailscale device name set DEVICE_ID NAME
omni authorize tailscale device tag set DEVICE_ID tag:prod
omni observe tailscale acl get --output acl.hujson
omni observe tailscale acl validate acl.hujson
omni observe tailscale acl preview --for tag:github
omni administer tailscale acl set acl.hujson --backup acl-before-change.hujson
omni observe tailscale key list
omni observe tailscale key get KEY_ID
omni observe tailscale dns get
omni observe tailscale user list
omni observe tailscale user get USER_ID
```

Device and user views intentionally return compact records. Device lists show
only the ID, hostname, OS, and last-seen time by default; `--details` adds
selection context without requiring a get-per-device loop. ACL download
always writes a new private file rather than placing a potentially large
tailnet ACL in terminal or agent context. Validate a proposed ACL through the
read-only Tailscale validation endpoint before applying it: failed validation
returns a nonzero exit status. Preview identifies the rules matching a source
identity without applying a change. ACL replacement validates the candidate,
requires a new private backup file, and uses an ETag guard. Tag replacement and
ACL replacement have their own high-impact effect paths.

## Capability discovery

Omni exposes a service-level operation catalog for both people and agents:

```bash
omni describe
omni describe trello
omni describe trello --format=json
omni describe observe trello card get --format=json
```

The service JSON includes stable `operation_id` values, action-first command tokens, effect metadata, summaries, optional notes, response descriptions, arguments, options, and credential requirements. This is intentionally shaped like MCP tool enumeration while remaining a local CLI contract; an eventual MCP adapter can synthesize its standalone tool description from the same fields, while the default terminal view is a human-oriented service manual.

## Library use

Go programs can embed Omni's command and discovery contracts without starting a
subprocess. The public package provides `omni.Operations`,
`omni.FindOperation`, and `omni.Run`; it preserves the normal local policy and
configuration behavior. See [library use](docs/library.md) and the runnable
[`examples/library`](examples/library) demonstration.

## Developing

Contributor workflow, testing, release process, GitHub Pages, installer maintenance, and SDLC policies are in [DEVELOPING.md](DEVELOPING.md).

## Policies

Use `OMNI_POLICY=read-only` or `omni --policy read-only …` to reject all non-observe commands locally. Other initial modes are `no-delete` and `unattended-safe`.

## Design

The project brief is in [universal-cli-project.md](universal-cli-project.md).

## License

BSD 3-Clause. See [LICENSE](LICENSE).
