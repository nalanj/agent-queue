package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	apiKey  = os.Getenv("AGENT_QUEUE_API_KEY")
	baseURL = os.Getenv("AGENT_QUEUE_URL")
)

func init() {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
}

var RootCmd = &cobra.Command{
	Use:   "agent-queue",
	Short: "A simple job queue CLI",
	Long:  `agent-queue is a CLI for interacting with the agent-queue job queue API.`,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
