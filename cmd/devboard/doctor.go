package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/mathewepstein/devboard/internal/agent"
	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/doctor"
	"github.com/mathewepstein/devboard/internal/secrets"
	"github.com/spf13/cobra"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "doctor",
		Short:         "Check config, tokens, Jira, GitHub, the agent, and the running page",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			ao, err := agentOptions()
			if err != nil {
				return err
			}
			checks := doctor.Run(cmd.Context(), doctor.Options{
				ConfigPath: path,
				Token:      secrets.Get,
				Source:     secrets.Source,
				InKeychain: secrets.InKeychain,
				Agent:      func() agent.Status { return agent.Inspect(ao) },
				Now:        time.Now,
			})
			width := 0
			for _, c := range checks {
				width = max(width, len(c.Name))
			}
			out := cmd.OutOrStdout()
			for _, c := range checks {
				fmt.Fprintf(out, "%-4s  %-*s  %s\n", c.Level, width, c.Name, c.Detail)
			}
			if doctor.Failed(checks) {
				return errors.New("doctor found problems")
			}
			return nil
		},
	}
}
