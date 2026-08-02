package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nalanj/agent-queue/client"
	"github.com/spf13/cobra"
)

var dequeueCmd = &cobra.Command{
	Use:   "dequeue",
	Short: "Dequeue (claim) the next pending job",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New()
		job, err := c.Dequeue()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if job == nil {
			return nil
		}

		b, _ := json.MarshalIndent(job, "", "  ")
		fmt.Println(string(b))
		return nil
	},
}

func init() {
	RootCmd.AddCommand(dequeueCmd)
}
