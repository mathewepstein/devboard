package secrets

import (
	"errors"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	Namespace    = "devboard"
	keychainUser = "token"
)

// Env overrides win over the keychain, first match used.
var EnvVars = map[string][]string{
	"jira":   {"DEVBOARD_JIRA_TOKEN", "ATLASSIAN_API_TOKEN"},
	"github": {"DEVBOARD_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"},
}

var ErrNotFound = errors.New("token not found in keychain")

func Key(service string) string { return Namespace + ":" + service }

func Set(service, token string) error {
	if service == "" || token == "" {
		return fmt.Errorf("service and token required")
	}
	if err := keyring.Set(Key(service), keychainUser, token); err != nil {
		return fmt.Errorf("store %s token in keychain: %w", service, err)
	}
	return nil
}

func Get(service string) (string, error) {
	for _, name := range EnvVars[service] {
		if v := os.Getenv(name); v != "" {
			return v, nil
		}
	}
	tok, err := keyring.Get(Key(service), keychainUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read %s token from keychain: %w", service, err)
	}
	return tok, nil
}

func Delete(service string) error {
	err := keyring.Delete(Key(service), keychainUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
