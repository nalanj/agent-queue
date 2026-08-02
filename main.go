package main

import (
	"log"

	"github.com/nalanj/agent-queue/db"
)

func main() {
	database, err := db.New("agent-queue.db")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	log.Println("agent-queue started")
}
