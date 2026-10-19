package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mathewepstein/devboard/internal/agent"
	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/doctor"
	"github.com/mathewepstein/devboard/internal/secrets"
	"github.com/spf13/cobra"
)

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the launchd agent that keeps the page running",
	}
	var wait time.Duration
	status := &cobra.Command{
		Use:   "status",
		Short: "Show whether the agent is loaded and when it last polled",
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := agentOptions()
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			deadline := time.Now().Add(wait)
			c := doctor.ServerCheck(cmd.Context(), doctor.Options{Now: time.Now}, cfg)
			for c.Level != doctor.OK && time.Now().Before(deadline) {
				time.Sleep(500 * time.Millisecond)
				c = doctor.ServerCheck(cmd.Context(), doctor.Options{Now: time.Now}, cfg)
			}
			out := cmd.OutOrStdout()
			s := agent.Inspect(o)
			fmt.Fprintf(out, "plist:  %s (installed: %t)\n", agent.PlistPath(o.Home), s.Installed)
			if s.Loaded {
				fmt.Fprintf(out, "agent:  %s, pid %d\n", s.State, s.PID)
			} else {
				fmt.Fprintln(out, "agent:  not loaded")
			}
			fmt.Fprintf(out, "server: %s %s\n", c.Level, c.Detail)
			return nil
		},
	}
	status.Flags().DurationVar(&wait, "wait", 0, "keep checking until the server has polled, up to this long")
	cmd.AddCommand(
		status,
		&cobra.Command{
			Use:   "install",
			Short: "Write the LaunchAgent plist and load it",
			RunE: func(cmd *cobra.Command, args []string) error {
				o, err := agentOptions()
				if err != nil {
					return err
				}
				if err := agent.Install(o); err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Loaded %s from %s\nLogs: %s\n", agent.Label, agent.PlistPath(o.Home), agent.LogPath(o.Home))
				warnEnvOnly(cmd.ErrOrStderr())
				return nil
			},
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "Unload the agent and remove its plist",
			RunE: func(cmd *cobra.Command, args []string) error {
				o, err := agentOptions()
				if err != nil {
					return err
				}
				if err := agent.Uninstall(o); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", agent.Label)
				return nil
			},
		},
	)
	return cmd
}

func agentOptions() (agent.Options, error) {
	exe, err := os.Executable()
	if err != nil {
		return agent.Options{}, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return agent.Options{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return agent.Options{}, err
	}
	o := agent.Options{Executable: exe, Home: home, UID: os.Getuid(), Runner: agent.Launchctl{}}
	if os.Getenv(config.EnvPathOverride) != "" {
		if o.ConfigOverride, err = config.Path(); err != nil {
			return agent.Options{}, err
		}
	}
	return o, nil
}

func warnEnvOnly(w io.Writer) {
	for _, svc := range []string{"jira", "github"} {
		if !secrets.InKeychain(svc) {
			fmt.Fprintf(w, "warning: no %s token in the keychain; the agent has no shell env, run `devboard init`\n", svc)
		}
	}
}
