package command

import (
	"strings"
	"testing"
)

// closedVocabularies records every fixed set of values Omni accepts from an
// operator, and who owns that set. The distinction is the one that bites: a
// vocabulary Omni owns can only ever reject values Omni never offered, while a
// vocabulary copied from a provider spec goes stale silently and starts
// rejecting input the provider already accepts. A stale set gating a write turns
// Omni into the obstacle between an operator and a working provider feature, and
// it presents as "Omni does not support this" rather than "Omni's copy is old".
//
// Every entry needs a policy:
//
//	omni       Omni's own vocabulary, mapped onto provider values internally.
//	pinned     Copied from a provider, and pinned by a test that proves every
//	           value obtainable from a read is accepted on the write path.
//	passthru   Not validated locally; the provider decides.
//
// A new closed vocabulary fails this test until it is classified here, so the
// next one cannot arrive unexamined.
var closedVocabularies = map[string]string{
	"--scope open|archived|all":       "omni",
	"--state complete|incomplete":     "omni",
	"--state enabled|disabled":        "omni",
	"--state authorized|unauthorized": "omni",
	// Trello's label palette: pinned by TestLabelPaletteEnumeratesExactlyWhatLabelColorAccepts,
	// and deliberately wider than the committed OpenAPI snapshot, whose Color enum
	// predates the subtle and bold shades.
	"--color COLOR": "pinned",
	// Trello decides which emoji are valid; unknown values are forwarded rather
	// than refused, so this cannot reject a working reaction.
	"--emoji EMOJI": "passthru",
}

func TestClosedVocabulariesAreClassified(t *testing.T) {
	valid := map[string]bool{"omni": true, "pinned": true, "passthru": true}
	seen := map[string]bool{}
	for _, d := range Registry {
		for _, option := range d.Options {
			key := option.Name + " " + option.Value
			if !strings.Contains(option.Value, "|") && option.Value != "COLOR" && option.Value != "EMOJI" {
				continue
			}
			policy, classified := closedVocabularies[key]
			if !classified {
				t.Fatalf("%q accepts the closed vocabulary %q with no recorded policy; classify it in closedVocabularies as omni, pinned, or passthru", d.Name(), key)
			}
			if !valid[policy] {
				t.Fatalf("closed vocabulary %q has unknown policy %q", key, policy)
			}
			seen[key] = true
		}
	}
	for key := range closedVocabularies {
		if !seen[key] {
			t.Fatalf("closedVocabularies records %q, which no command offers any more; remove it", key)
		}
	}
}

func TestEffectIsFirstCommandToken(t *testing.T) {
	for _, d := range Registry {
		tokens := d.Tokens()
		if tokens[0] != string(d.Effect) {
			t.Fatalf("%q does not begin with effect", d.Name())
		}
		if d.Effect == Observe && !d.UnattendedOK {
			t.Fatalf("observe command %q should be unattended-safe", d.Name())
		}
	}
}

func TestRegistrySafetyContractInvariants(t *testing.T) {
	for _, d := range Registry {
		if err := d.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefinitionRejectsUnsafeContractClaims(t *testing.T) {
	base := Definition{Effect: Update, Path: []string{"test", "thing"}, Summary: "Test.", Response: "Test result."}
	unsafe := base
	unsafe.UnattendedOK = true
	if err := unsafe.Validate(); err == nil {
		t.Fatal("mutating unattended command was accepted")
	}
	reversible := base
	reversible.Reversible = true
	if err := reversible.Validate(); err == nil {
		t.Fatal("reversible mutation without a reversal was accepted")
	}
	incorrect := base
	incorrect.Reversal = "not applicable"
	if err := incorrect.Validate(); err == nil {
		t.Fatal("non-reversible command with reversal was accepted")
	}
	emptyNote := base
	emptyNote.Notes = []string{""}
	if err := emptyNote.Validate(); err == nil {
		t.Fatal("empty note was accepted")
	}
}
