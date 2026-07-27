package omni

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestOperationsReturnsIndependentDescriptions(t *testing.T) {
	operations := Operations()
	if len(operations) == 0 {
		t.Fatal("Operations returned no runnable operations")
	}
	operation, err := FindOperation([]string{"observe", "trello", "board", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if operation.ID != "omni.observe.trello.board.list" {
		t.Fatalf("operation ID = %q", operation.ID)
	}
	operations[0].Path[0] = "changed"
	again := Operations()
	if again[0].Path[0] == "changed" {
		t.Fatal("Operations exposed the registry's mutable path")
	}
}

func TestRunEmbedsCLI(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "trello"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Trello") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
