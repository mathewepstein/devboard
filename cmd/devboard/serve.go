package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/poll"
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
			h, err := server.New(cfg, store)
			if err != nil {
				return err
			}
			if token := githubToken(); token != "" {
				go poll.New(cfg, store, github.NewClient(token)).Run(cmd.Context())
			} else {
				s := store.Get()
				s.Errors.GitHub = "GH_TOKEN not set"
				store.Set(s)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: GH_TOKEN not set; GitHub polling disabled")
			}
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
