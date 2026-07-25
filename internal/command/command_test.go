package command

import "testing"

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
