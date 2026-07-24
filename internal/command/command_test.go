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
