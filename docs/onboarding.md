# Onboarding a service

This guide turns provider work into a repeatable product-development loop. The
goal is a small, comprehensible CLI contract—not a thin wrapper around every
endpoint in a vendor API.

## Start with source material

Create `docs/SERVICE.md` before implementing a client. Link the vendor's
official API reference or OpenAPI/Swagger definition, authentication guide,
scope/permission model, and any relevant policy or rate-limit documentation.
Record exact endpoints only after checking the primary documentation.

Keep a dated snapshot of the machine-readable specification in `docs/` when the
provider publishes one. It settles questions that prose and experiment cannot:
which query parameters exist, which status codes a mutation can return, and
which scope governs each part of a response. Never conclude that a provider
lacks a capability because a guessed endpoint returned `404`. A wrong path name
and a missing feature are indistinguishable from the outside, so confirm absence
against the specification rather than against a probe.

## Scope by demonstrated need

Start with the smallest operations that solve a real workflow. Do not mirror a
provider's whole API or advertise aspirational operations. A discovery listing
is an executable contract: every listed command must work.

Add operations incrementally when use demonstrates a gap. Keep service-specific
notes about rejected scope and why it was deferred; this makes later expansion
intentional rather than accidental.

## Design the command before the request

Choose the leftmost effect first: `observe`, `create`, `update`, `move`,
`archive`, `delete`, `authorize`, or `administer`. It must name the highest
impact action. An option can filter, select a format, or add response detail;
it must never turn an approved observation into a mutation.

Use separate commands for materially different effects. For example, policy
download is `observe`, while policy replacement is `administer`; tag replacement
is `authorize`, not an option on device observation.

Document arguments, options, response shape, and credential requirement in the
command registry. Human discovery should read like a compact manual; JSON
discovery can carry stable metadata for agents.

Treat that registry as executable, agent-facing contract data rather than
descriptive prose. Add tests for every safety claim it carries. In particular,
unattended-safe commands must remain observation-only or otherwise fail closed,
and a mutating command marked reversible must name the concrete recovery path.
If the implementation lacks an unarchive, undo, or restore operation, do not
advertise reversibility merely because the remote provider happens to retain an
object. These checks prevent metadata from drifting into promises an agent may
reasonably trust.

The same discipline applies to option descriptions. An option that forwards a
provider parameter must describe what that parameter actually does, verified
against the specification rather than inferred from its name. A flag named
`--all` that widens which records are owned, not which are valid, will be read
as the latter unless the description says otherwise, and the reader will go
looking for records the endpoint cannot return.

## Preserve concurrency and recovery guarantees

Before implementing a remote replacement or update, look for provider version
guards such as ETags, revisions, generation numbers, or conditional-write
headers. Use them when available: fetch the current object and its version as
one snapshot, retain a private recovery copy when the change is destructive,
then make the mutation conditional on that exact version. Treat a rejected
precondition (for example HTTP `412 Precondition Failed`) as a safe conflict,
not a reason to retry blindly.

Document the mechanism, not merely advice. If a command is marked reversible,
state precisely what makes it reversible—such as a required pre-change backup
and the command needed to restore it. Never describe a destructive replacement
as reversible solely because an operator could have remembered to save a copy.

## Make failure unambiguous

Exit status is part of the contract. A command that reports a provider's
rejection must exit nonzero, so that `verify && mutate` composes correctly in a
shell and so that an unattended caller learns the outcome without interpreting
prose. A validation command that returns success while printing an error message
is worse than one that does not exist, because it invites the pipeline that
depends on it.

Give success and failure the same output shape, distinguished by a stable field
rather than by the presence or absence of a message. Write structured output to
stdout and the human-readable explanation to stderr, so that both a person and a
parser can consume the same run.

Distinguish an empty result from a refused one. An empty collection that
actually means "insufficient scope," "only active records are returned," or
"filtered by a flag you passed" will be read as "nothing exists," and that
misreading can end an investigation prematurely. Say which of these happened,
either in the response or in the documented behavior of the command. When a
provider refuses for permission reasons, name the specific scope or grant
required; an accurate but unactionable permission error relocates the guesswork
instead of removing it.

## Minimize response tokens

Default output should identify the next useful action, not reproduce the API.
For a list, include the identifier needed for `get` plus a few selection
signals. Add `--details` only when it prevents an agent from needing a
list-then-get loop, and make its additional fields explicit. Do not create a
raw-output escape hatch by default.

When one endpoint returns more than one kind of record, shape each variant
deliberately. A compaction routine written for the common record will silently
drop the fields that distinguish the others, and those are frequently the exact
fields the command exists to surface. Compaction is a decision about what a
reader needs, so it must be reconsidered whenever a new record type starts
flowing through the same path.

For large or sensitive documents, write a new private local file instead of
printing the full value. Never overwrite a file implicitly. Explain this local
side effect in discovery documentation even when the remote operation is an
`observe` action.

## Model configuration and credentials deliberately

Put ordinary, mockable endpoint defaults in the configuration registry. Store
user-provided secrets only in the secured credential file. Treat generated,
short-lived secrets separately from user-managed credentials, with their own
secured ephemeral location and expiry behavior.

Support credential removal through configuration commands; users should not
need to hand-edit TOML. Never print secret values, including in transport
errors, setup output, or diagnostics.

## Implement and verify locally

Use fixed reviewed request shapes—never an arbitrary method/path/body command.
Use standard-library HTTP and in-memory transports for tests. Cover request
method/path/body/authentication, credential redaction, compact output, option
parsing, and local file permissions.

Run the normal local loop before a commit:

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
go build -o ./bin/omni ./cmd/omni
./bin/omni describe SERVICE
```

Only use real credentials for explicitly requested, minimal manual refinement.
For a live service, begin with a known read-only command and never run a
mutation merely to test the integration.

Local tests confirm that the code does what it was written to do. They cannot
confirm that the registry describes what the command actually does, and that gap
is where the costly defects live: a safety claim that no longer holds, an option
description that outran the provider, a success status returned for a rejected
request. Re-verify safety claims against the live provider when the surrounding
behavior changes, exercising failure paths with deliberately invalid input
rather than only confirming the successful one. A guard that has never been seen
to refuse anything has not been tested.

When a mutation must be proven against a live provider, look for an input whose
success and failure are equally harmless—replaying an object's current contents,
or supplying a stale version guard—so that the observable difference is the
status code rather than a change to the caller's state.
