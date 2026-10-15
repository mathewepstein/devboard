package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/initflow"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var template, force, noEnv bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactive first-run setup (or --template to write a commented config)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("config already exists at %s (re-run with --force to overwrite)", path)
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if template {
				if err := config.EnsureDir(path); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(config.Template()), 0o600); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote config template to %s\nEdit it, then store tokens with `devboard init` or set GH_TOKEN / ATLASSIAN_API_TOKEN.\n", path)
				return nil
			}
			return initflow.Run(cmd.Context(), initflow.Options{Out: cmd.OutOrStdout(), NoEnv: noEnv})
		},
	}
	cmd.Flags().BoolVar(&template, "template", false, "write a commented config template instead of prompting")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config")
	cmd.Flags().BoolVar(&noEnv, "no-env", false, "ignore ATLASSIAN_EMAIL / ATLASSIAN_API_TOKEN / GH_TOKEN / GITHUB_ORG and prompt for everything")
	return cmd
}
