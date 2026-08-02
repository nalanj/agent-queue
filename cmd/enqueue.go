package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/nalanj/agent-queue/client"
	"github.com/spf13/cobra"
)

var (
	enqueueBody      string
	enqueueTags      string
)

var enqueueCmd = &cobra.Command{
	Use:   "enqueue [dedupe-key]",
	Short: "Enqueue a new job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dedupeKey := args[0]
		if dedupeKey == "" {
			return fmt.Errorf("dedupe-key is required")
		}

		if enqueueBody == "" {
			return fmt.Errorf("body is required (--body)")
		}

		var tags []string
		if enqueueTags != "" {
			tags = strings.Split(enqueueTags, ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
			}
		}

		c := client.New()
		job, err := c.Enqueue(dedupeKey, enqueueBody, tags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Enqueued job %d\n", job.ID)
		return nil
	},
}

func init() {
	enqueueCmd.Flags().StringVar(&enqueueBody, "body", "", "Job body (required)")
	enqueueCmd.Flags().StringVar(&enqueueTags, "tags", "", "Comma-separated tags")
	RootCmd.AddCommand(enqueueCmd)
}
