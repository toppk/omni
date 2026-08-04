package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitializeSecuresExistingCredentialPaths(t *testing.T) {
	root := t.TempDir()
	p := Paths{
		Root:        root,
		Settings:    filepath.Join(root, "settings.toml"),
		Credentials: filepath.Join(root, "credentials", "credentials.toml"),
	}
	if err := os.MkdirAll(filepath.Dir(p.Credentials), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Credentials, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(p); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(filepath.Dir(p.Credentials))
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(p.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("credentials directory mode = %o, want 0700", got)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("credentials file mode = %o, want 0600", got)
	}
}

func TestLoadEphemeralTailscaleTokenMetadataOmitsSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ephemeral", "credentials.toml")
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	if err := StoreEphemeralTailscaleToken(path, "must-not-return", expiresAt); err != nil {
		t.Fatal(err)
	}
	metadata, err := LoadEphemeralTailscaleTokenMetadata(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.Cached || !metadata.Valid || !metadata.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestSetWritesAndUpdatesRegistryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte("policy = \"default\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "trello.default-board-id", "board-one"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "trello.default-board-id", "board-two"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "policy = \"default\"\n\n[trello]\ndefault-board-id = \"board-two\"\n"
	if got := string(b); got != want {
		t.Fatalf("settings = %q, want %q", got, want)
	}
}

func TestDeleteRemovesOnlyRequestedRegistryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	if err := os.WriteFile(path, []byte("[tailscale]\napi-key = \"token\"\nclient-secret = \"secret\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	deleted, err := Delete(path, "tailscale.api-key")
	if err != nil || !deleted {
		t.Fatalf("deleted=%t err=%v", deleted, err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "[tailscale]\nclient-secret = \"secret\"\n" {
		t.Fatalf("content=%q err=%v", b, err)
	}
}
