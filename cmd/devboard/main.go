// Command devboard: local repo and ticket status page.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	serve := serveCmd()
	root := &cobra.Command{
		Use:     "devboard",
		Short:   "Local status page for your repos and tickets",
		Version: version,
		RunE:    serve.RunE,
	}
	root.Flags().AddFlagSet(serve.Flags())
	root.AddCommand(serve, initCmd())
	return root
}
