# Trello test fixtures

## label-colors.json

`observe trello label list --format json` captured from a disposable Trello
board on 2026-08-04. Every value in it is a color Trello actually returned, not
a value composed by hand or copied out of the vendored OpenAPI snapshot.

It exists to pin one property: **every label color obtainable from a read must be
accepted by the write validator.** `labelColor` validates `--color` on `label
create` and `label set`, and it deliberately accepts more than
`docs/trello-openapi-v3-snapshot-20260724.json` documents, because that
snapshot's `Color` enum predates Trello's subtle and bold shades. Validating
against the snapshot instead would report `green_dark` faithfully on a read and
then reject it on a write, breaking recreation for any label colored through the
Trello interface.

**The colors here are deliberate and must not be normalized away when this file
is refreshed.** The capture has to keep covering all four tiers:

| Tier | Value in this capture | Provenance |
|---|---|---|
| unshaded | `green`, plus `blue`, `orange`, `purple`, `red`, `yellow` | board defaults |
| subtle (`_light`) | `green_light`, `sky_light` | set through Omni |
| bold (`_dark`) | `green_dark`, `black_dark` | `green_dark` set through the Trello **UI** |
| colorless (`null`) | one label | set through Omni as `--color none` |

Two hues carry shades, not one, so the pin cannot pass on a green-specific
accident. `green_dark` came from the UI specifically: UI-set shades are the exact
provenance a stale validator would reject, so a capture without one cannot prove
the property it exists to prove.

`TestLiveCaptureColorsAreAcceptedByTheWriteValidator` asserts that coverage
rather than trusting this note. A refreshed capture that loses a shade tier, or
that contains nothing absent from the snapshot's ten-hue enum, fails loudly
instead of passing by construction.

Label names describe what each label tests. The board is disposable and holds no
real work, so nothing here is pseudonymized and the label IDs are real.
