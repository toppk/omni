package policy

import (
	"github.com/toppk/omni/internal/command"
	"testing"
)

func TestReadOnlyRejectsMutation(t *testing.T) {
	d, err := command.Find([]string{"delete", "trello", "card", "delete"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Allows("read-only", d); err == nil {
		t.Fatal("delete unexpectedly allowed")
	}
}
