package main

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/jira"
	"github.com/mathewepstein/devboard/internal/poll"
	"github.com/mathewepstein/devboard/internal/secrets"
	"github.com/mathewepstein/devboard/internal/seen"
	"github.com/mathewepstein/devboard/internal/server"
	"github.com/mathewepstein/devboard/internal/state"
	"github.com/spf13/cobra"
)

func serveCmd() *cobra.Command {
	var port int
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the status page on 127.0.0.1 (default command)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid config: %w", err)
			}
			if cmd.Flags().Changed("port") {
				cfg.Poll.Port = port
			}
			if cmd.Flags().Changed("interval") {
				cfg.Poll.Interval.Duration = interval
			}
			store := state.NewStore()
			dataDir, err := config.DataDir()
			if err != nil {
				return err
			}
			sn, err := seen.Open(filepath.Join(dataDir, "seen.json"))
			if err != nil {
				return err
			}
			h, err := server.New(cfg, store, sn)
			if err != nil {
				return err
			}
			ghToken, ghErr := secrets.Get("github")
			jiraToken, jiraErr := secrets.Get("jira")
			p := poll.New(cfg, store, github.NewClient(ghToken))
			p.Seen = sn
			s := store.Get()
			if ghErr != nil {
				s.Errors.GitHub = "GitHub token: " + ghErr.Error()
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", s.Errors.GitHub)
			}
			if jiraErr == nil {
				p.Jira = jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, jiraToken)
			} else {
				s.Errors.Jira = "Jira token: " + jiraErr.Error()
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", s.Errors.Jira)
			}
			store.Set(s)
			go p.Run(cmd.Context())
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Poll.Port))
			fmt.Fprintf(cmd.OutOrStdout(), "devboard on http://%s (poll %s)\n", addr, cfg.Poll.Interval)
			srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
			return srv.ListenAndServe()
		},
	}
	cmd.Flags().IntVar(&port, "port", config.DefaultPort, "listen port")
	cmd.Flags().DurationVar(&interval, "interval", config.DefaultInterval, "poll interval")
	return cmd
}
