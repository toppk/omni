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

func TestEncodeTextProjectsCardCollectionsForScanning(t *testing.T) {
	var out bytes.Buffer
	value := map[string]any{"cards": []map[string]any{
		{"id": "card-1", "idList": "list-1", "name": "Review workflow", "desc": strings.Repeat("long ", 100), "pos": 140737488453632.0, "due": "2026-08-01", "idMembers": []any{"member-1"}, "labels": []any{map[string]any{"name": "review"}}, "badges": map[string]any{"checkItems": 12, "checkItemsChecked": 3, "comments": 9}},
	}}
	if err := Encode(&out, Text, value); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAME", "PROGRESS", "LABELS", "MEMBERS", "3/12", "review", "1 assigned"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "long long") || strings.Contains(out.String(), "140737") || strings.Contains(out.String(), "COMMENTS") {
		t.Fatalf("card scan retained detail:\n%s", out.String())
	}
}
