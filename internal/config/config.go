package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Jira   Jira   `toml:"jira"`
	GitHub GitHub `toml:"github"`
	Repos  []Repo `toml:"repos"`
	Poll   Poll   `toml:"poll"`
}

type Jira struct {
	BaseURL        string `toml:"base_url"`
	Email          string `toml:"email"`
	Project        string `toml:"project"`
	AccountID      string `toml:"account_id"`
	DisplayName    string `toml:"display_name"`
	PickedUpStatus string `toml:"picked_up_status"`
	Scope          Scope  `toml:"scope"`
}

// One kind set at most; empty = whole project.
type Scope struct {
	Labels  []string `toml:"labels"`
	BoardID int      `toml:"board_id"`
	JQL     string   `toml:"jql"`
}

type GitHub struct {
	Login string `toml:"login"`
	Org   string `toml:"org"`
}

type Repo struct {
	Name   string `toml:"name"`
	Repo   string `toml:"repo"`
	Prefix string `toml:"prefix"`
}

type Poll struct {
	Interval Duration `toml:"interval"`
	Port     int      `toml:"port"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.Duration.String()), nil }

const (
	DefaultInterval = 60 * time.Second
	DefaultPort     = 8765
)

func (c *Config) applyDefaults() {
	if c.Poll.Interval.Duration == 0 {
		c.Poll.Interval.Duration = DefaultInterval
	}
	if c.Poll.Port == 0 {
		c.Poll.Port = DefaultPort
	}
}

func (c *Config) Validate() error {
	var errs []string
	if c.Jira.Project == "" {
		errs = append(errs, "jira.project is required")
	}
	if len(c.Repos) == 0 {
		errs = append(errs, "at least one [[repos]] entry is required")
	}
	for i, r := range c.Repos {
		if r.Repo == "" {
			errs = append(errs, fmt.Sprintf("repos[%d].repo is required", i))
		}
	}
	kinds := 0
	if len(c.Jira.Scope.Labels) > 0 {
		kinds++
	}
	if c.Jira.Scope.BoardID != 0 {
		kinds++
	}
	if c.Jira.Scope.JQL != "" && c.Jira.Scope.BoardID == 0 {
		kinds++
	}
	if kinds > 1 {
		errs = append(errs, "jira.scope: set only one of labels, board_id, jql")
	}
	if c.Jira.Scope.BoardID != 0 && c.Jira.Scope.JQL == "" {
		errs = append(errs, "jira.scope.board_id set without its stored jql")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// JQL fragment to AND in; "" = no narrowing.
func (s Scope) Clause() string {
	switch {
	case len(s.Labels) > 0:
		q := make([]string, len(s.Labels))
		for i, l := range s.Labels {
			q[i] = `"` + strings.ReplaceAll(l, `"`, `\"`) + `"`
		}
		return "labels IN (" + strings.Join(q, ", ") + ")"
	case s.JQL != "":
		return "(" + s.JQL + ")"
	}
	return ""
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(p)
}

func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("config file not found at %s (run `devboard init`): %w", path, err)
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	return c.SaveTo(p)
}

// SaveTo writes atomically via a same-directory temp file and rename.
func (c *Config) SaveTo(path string) error {
	if err := EnsureDir(path); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".devboard-config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	fail := func(step string, err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%s: %w", step, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fail("chmod temp config", err)
	}
	if err := toml.NewEncoder(tmp).Encode(c); err != nil {
		return fail("encode config", err)
	}
	if err := tmp.Sync(); err != nil {
		return fail("fsync temp config", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func writeFile(path, content string) error {
	if err := EnsureDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
