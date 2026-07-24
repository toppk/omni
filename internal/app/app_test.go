package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/toppk/omni/internal/config"
)

func TestSetupTrelloShowsSingleConfigurationCommand(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"setup", "trello"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN") {
		t.Fatalf("unexpected setup output: %s", text)
	}
	if !strings.Contains(text, "developer.atlassian.com") {
		t.Fatal("missing credential URL")
	}
}

func TestConfigureTrelloHelpDoesNotInitializeFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"configure", "trello", "--help"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--api-token") {
		t.Fatal("missing Trello options")
	}
}

func TestConfigureTrelloStoresSettingsAndSecrets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	err := Run(context.Background(), []string{"configure", "trello", "--default-board", "board-id", "--api-key", "key-value", "--api-token", "token-value"}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	board, err := config.Get(paths.Settings, "trello.default-board-id")
	if err != nil {
		t.Fatal(err)
	}
	if board != "board-id" {
		t.Fatalf("board = %q", board)
	}
	credentials, err := config.LoadTrelloCredentials(paths.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.APIKey != "key-value" || credentials.Token != "token-value" {
		t.Fatalf("credentials were not stored")
	}
	if strings.Contains(out.String(), "key-value") || strings.Contains(out.String(), "token-value") {
		t.Fatal("secret appeared in output")
	}
}

func TestVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-V"}} {
		var out bytes.Buffer
		if err := Run(context.Background(), args, &out, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "omni "+Version+"\n"; got != want {
			t.Fatalf("%v: got %q, want %q", args, got, want)
		}
	}
}
