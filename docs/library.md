# Using Omni as a Go library

Omni can be embedded by importing `github.com/toppk/omni`. The public package
deliberately exposes two small contracts:

- `Operations` and `FindOperation` provide typed, provider-neutral capability
  discovery. Each operation retains its action-first `Effect`, safety metadata,
  arguments, options, and credential requirement.
- `Run` executes the same command contract as the `omni` binary. Its argument
  slice excludes the binary name and its output streams are supplied by the
  caller.

```go
operation, err := omni.FindOperation([]string{"observe", "trello", "board", "list"})
if err != nil { /* handle unknown operation */ }

err = omni.Run(ctx, []string{"describe", "trello", "--format", "json"}, out, errOut)
```

`Run` uses the normal local configuration, credentials, environment, and
policy behavior. In particular, embedding does not bypass `OMNI_POLICY` or the
action-first effect checks. The complete runnable example is in
[`examples/library`](../examples/library).

## Declarative operation definitions

The operation registry currently lives in Go because it is the executable
source of truth for command matching, discovery, and policy checks. That is
not a reason for its *metadata* to remain handwritten Go.

The recommended next step is a constrained declarative operation schema, not a
general-purpose DSL. JSON is a good initial format because Omni has no external
dependencies and can parse it with the Go standard library. TOML or YAML can
be added later only if their authoring benefits justify a dependency.

```json
{
  "effect": "observe",
  "path": ["inventory", "asset", "list"],
  "summary": "List managed assets.",
  "response_description": "A collection of compact asset records.",
  "cardinality": "many",
  "reversible": true,
  "credentials": "inventory",
  "unattended_ok": true
}
```

The loader should decode this format into the same validated operation model
used today. Provider behavior remains Go code registered explicitly against an
operation ID; a data file must never be able to attach arbitrary execution or
change its effect through an option. This preserves Omni's central safety
property while making service catalogs easier to author, review, generate from
an API specification, and eventually distribute with an external plugin.

Before migrating the existing registry, add schema fixtures and parity tests so
that generated discovery, command matching, and policy decisions exactly match
the current definitions. A plugin system can then consume the same validated
schema and use a separate executable for provider execution.
