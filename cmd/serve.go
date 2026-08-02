package cmd

import (
	"log"
	"net/http"
	"os"

	"github.com/nalanj/agent-queue/db"
	"github.com/nalanj/agent-queue/handlers"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the agent-queue server",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := os.Getenv("AGENT_QUEUE_API_KEY")
		if apiKey == "" {
			log.Fatal("AGENT_QUEUE_API_KEY is required")
		}

		database, err := db.NewFromEnv()
		if err != nil {
			return err
		}
		defer database.Close()

		if err := database.Migrate(); err != nil {
			return err
		}

		h := handlers.New(database, apiKey)

		port := os.Getenv("AGENT_QUEUE_PORT")
		if port == "" {
			port = "8080"
		}

		log.Printf("agent-queue starting on :%s", port)
		return http.ListenAndServe(":"+port, h)
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
