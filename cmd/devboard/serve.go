package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/jira"
	"github.com/mathewepstein/devboard/internal/poll"
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
			p := poll.New(cfg, store, github.NewClient(githubToken()))
			p.Seen = sn
			s := store.Get()
			if githubToken() == "" {
				s.Errors.GitHub = "GH_TOKEN not set"
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: GH_TOKEN not set")
			}
			if jt := os.Getenv("ATLASSIAN_API_TOKEN"); jt != "" {
				p.Jira = jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, jt)
			} else {
				s.Errors.Jira = "ATLASSIAN_API_TOKEN not set"
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: ATLASSIAN_API_TOKEN not set")
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

func githubToken() string {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
