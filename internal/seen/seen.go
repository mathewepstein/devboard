package seen

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.Mutex
	ids  map[string]time.Time
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, ids: map[string]time.Time{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file struct {
		IDs map[string]time.Time `json:"ids"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if file.IDs != nil {
		s.ids = file.IDs
	}
	return s, nil
}

func (s *Store) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ids[id]
	return ok
}

func (s *Store) Mark(ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, id := range ids {
		if id != "" {
			s.ids[id] = now
		}
	}
	return s.save()
}

// Drop ids not in live.
func (s *Store) Prune(live map[string]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for id := range s.ids {
		if !live[id] {
			delete(s.ids, id)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.save()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		IDs map[string]time.Time `json:"ids"`
	}{s.ids}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
