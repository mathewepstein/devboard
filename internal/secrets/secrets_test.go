package secrets

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSource(t *testing.T) {
	keyring.MockInit()
	for _, v := range EnvVars["github"] {
		t.Setenv(v, "")
	}
	if src := Source("github"); src != "" {
		t.Fatalf("want none, got %q", src)
	}
	if err := Set("github", "kc"); err != nil {
		t.Fatal(err)
	}
	if src := Source("github"); src != "keychain" {
		t.Fatalf("want keychain, got %q", src)
	}
	t.Setenv("GH_TOKEN", "env")
	if src := Source("github"); src != "env:GH_TOKEN" {
		t.Fatalf("want env:GH_TOKEN, got %q", src)
	}
}

func TestInKeychain(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GH_TOKEN", "env")
	if InKeychain("github") {
		t.Fatal("env must not count as keychain")
	}
	if err := Set("github", "kc"); err != nil {
		t.Fatal(err)
	}
	if !InKeychain("github") {
		t.Fatal("want keychain hit")
	}
}
