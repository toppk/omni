package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeTextRendersCollectionAsTable(t *testing.T) {
	var out bytes.Buffer
	value := map[string]any{"boards": []map[string]any{
		{"id": "board-1", "name": "Research"},
		{"id": "board-2", "name": "Operations"},
	}}
	if err := Encode(&out, Text, value); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BOARDS (2)", "ID", "NAME", "board-1", "Research"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, out.String())
		}
	}
}

func TestEncodeJSONRemainsCanonical(t *testing.T) {
	var out bytes.Buffer
	if err := Encode(&out, JSON, map[string]any{"id": "board-1"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "{\"id\":\"board-1\"}\n"; got != want {
		t.Fatalf("JSON = %q, want %q", got, want)
	}
}
