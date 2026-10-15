package seen

import (
	"path/filepath"
	"testing"
)

func TestMarkHasPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Has("a") {
		t.Fatal("fresh store must be empty")
	}
	if err := s.Mark("a", "b"); err != nil {
		t.Fatal(err)
	}
	if !s.Has("a") || !s.Has("b") || s.Has("c") {
		t.Fatal("mark not applied")
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Has("a") || !again.Has("b") {
		t.Fatal("marks did not survive reopen")
	}
}

func TestPruneKeepsOnlyLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, _ := Open(path)
	_ = s.Mark("a", "b", "c")
	if err := s.Prune(map[string]bool{"a": true, "zzz": true}); err != nil {
		t.Fatal(err)
	}
	if !s.Has("a") || s.Has("b") || s.Has("c") {
		t.Fatal("prune wrong")
	}
	again, _ := Open(path)
	if again.Has("b") {
		t.Fatal("prune not persisted")
	}
}

func TestOpenCreatesParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "seen.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Mark("x"); err != nil {
		t.Fatal(err)
	}
}
