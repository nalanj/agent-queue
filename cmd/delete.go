package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nalanj/agent-queue/client"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <job-id>",
	Short: "Delete a job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid job ID: %s", args[0])
		}

		c := client.New()
		if err := c.Delete(jobID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Deleted job %d\n", jobID)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(deleteCmd)
}
