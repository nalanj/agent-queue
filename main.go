package main

import (
	"log"
	"net/http"
	"os"

	"github.com/nalanj/agent-queue/db"
	"github.com/nalanj/agent-queue/handlers"
)

func main() {
	apiKey := os.Getenv("AGENT_QUEUE_API_KEY")
	if apiKey == "" {
		log.Fatal("AGENT_QUEUE_API_KEY is required")
	}

	database, err := db.NewFromEnv()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	h := handlers.New(database, apiKey)

	port := os.Getenv("AGENT_QUEUE_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("agent-queue starting on :%s", port)
	if err := http.ListenAndServe(":"+port, h); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
