package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nalanj/agent-queue/client"
	"github.com/spf13/cobra"
)

var extendCmd = &cobra.Command{
	Use:   "extend <job-id>",
	Short: "Extend the claim on a job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid job ID: %s", args[0])
		}

		c := client.New()
		job, err := c.Extend(jobID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Extended job %d (claimed_at: %s)\n", job.ID, job.ClaimedAt)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(extendCmd)
}
