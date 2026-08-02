package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nalanj/agent-queue/client"
	"github.com/spf13/cobra"
)

var (
	listPage   int
	listLimit  int
	listStatus string
	listTag    string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New()
		result, err := c.List(listPage, listLimit, listStatus, listTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Page %d of %d (total: %d)\n\n", result.Page, result.TotalPages, result.Total)

		if len(result.Jobs) == 0 {
			fmt.Println("No jobs found")
			return nil
		}

		for _, job := range result.Jobs {
			b, _ := json.MarshalIndent(job, "", "  ")
			fmt.Println(string(b))
			fmt.Println("---")
		}

		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&listPage, "page", 1, "Page number")
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "Items per page")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status")
	listCmd.Flags().StringVar(&listTag, "tag", "", "Filter by tag")
	RootCmd.AddCommand(listCmd)
}
