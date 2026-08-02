package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/nalanj/agent-queue/db"
	"github.com/nalanj/agent-queue/handlers"
)

func TestIntegration(t *testing.T) {
	// Use temp database
	tmp := t.TempDir() + "/integration.db"
	os.Setenv("AGENT_QUEUE_DB_PATH", tmp)
	defer os.Remove(tmp)

	// Create and migrate DB
	testDB, err := db.New(tmp)
	if err != nil {
		t.Fatalf("Failed to create db: %v", err)
	}
	if err := testDB.Migrate(); err != nil {
		t.Fatalf("Failed to migrate db: %v", err)
	}

	// Create handler
	h := handlers.New(testDB, "test-key")

	// Start test server
	server := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	defer server.Close()

	client := server.Client()

	doRequest := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
		req.Header.Set("X-API-Key", "test-key")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		return resp
	}

	t.Run("full job lifecycle", func(t *testing.T) {
		// 1. Enqueue
		resp := doRequest("POST", "/jobs", `{"dedupe_key": "job-1", "body": "process me", "tags": ["test"]}`)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Enqueue status = %d, want %d", resp.StatusCode, http.StatusCreated)
		}

		var job map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&job)

		jobID := int(job["id"].(float64))
		if job["status"] != "pending" {
			t.Errorf("Initial status = %q, want %q", job["status"], "pending")
		}

		// 2. Dequeue
		resp = doRequest("POST", "/jobs/dequeue", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Dequeue status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var dequeueResp struct {
			Job     map[string]interface{} `json:"job"`
			Message string                  `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&dequeueResp)

		if dequeueResp.Job == nil {
			t.Fatal("Dequeue should return a job")
		}
		if dequeueResp.Job["status"] != "processing" {
			t.Errorf("Claimed status = %q, want %q", dequeueResp.Job["status"], "processing")
		}

		// 3. Extend
		resp = doRequest("POST", "/jobs/"+strconv.Itoa(jobID)+"/extend", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Extend status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		// 4. Delete
		resp = doRequest("DELETE", "/jobs/"+strconv.Itoa(jobID), "")
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("Delete status = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		// First enqueue
		resp := doRequest("POST", "/jobs", `{"dedupe_key": "dedupe-test", "body": "first"}`)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("First enqueue status = %d, want %d", resp.StatusCode, http.StatusCreated)
		}

		var firstJob map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&firstJob)

		// Second enqueue with same dedupe_key
		resp = doRequest("POST", "/jobs", `{"dedupe_key": "dedupe-test", "body": "second"}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Second enqueue (dedupe) status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var secondJob map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&secondJob)

		if firstJob["id"] != secondJob["id"] {
			t.Errorf("Dedupe should return same ID, got %v and %v", firstJob["id"], secondJob["id"])
		}
		if secondJob["body"] != "first" {
			t.Errorf("Dedupe should return original body, got %q", secondJob["body"])
		}
	})

	t.Run("pagination and filtering", func(t *testing.T) {
		// Create 5 jobs
		for i := 0; i < 5; i++ {
			doRequest("POST", "/jobs", fmt.Sprintf(`{"dedupe_key": "page-test-%d", "body": "job %d", "tags": ["batch"]}`, i, i))
		}

		// List all
		resp := doRequest("GET", "/jobs", "")
		var listResp struct {
			Jobs       []map[string]interface{} `json:"jobs"`
			Total      int                     `json:"total"`
			TotalPages int                     `json:"total_pages"`
		}
		json.NewDecoder(resp.Body).Decode(&listResp)

		if listResp.Total < 5 {
			t.Errorf("Total jobs = %d, want >= 5", listResp.Total)
		}

		// Paginate
		resp = doRequest("GET", "/jobs?page=1&limit=2", "")
		json.NewDecoder(resp.Body).Decode(&listResp)

		if len(listResp.Jobs) != 2 {
			t.Errorf("Page len = %d, want %d", len(listResp.Jobs), 2)
		}
		if listResp.TotalPages < 3 {
			t.Errorf("TotalPages = %d, want >= 3", listResp.TotalPages)
		}
	})

	t.Run("authentication", func(t *testing.T) {
		// No key
		req, _ := http.NewRequest("GET", server.URL+"/jobs", nil)
		resp, _ := client.Do(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("No key status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}

		// Wrong key
		req, _ = http.NewRequest("GET", server.URL+"/jobs", nil)
		req.Header.Set("X-API-Key", "wrong-key")
		resp, _ = client.Do(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Wrong key status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("empty queue", func(t *testing.T) {
		// Delete all jobs first
		resp := doRequest("GET", "/jobs", "")
		var listResp struct {
			Jobs []map[string]interface{} `json:"jobs"`
		}
		json.NewDecoder(resp.Body).Decode(&listResp)

		for _, job := range listResp.Jobs {
			id := int(job["id"].(float64))
			doRequest("DELETE", "/jobs/"+strconv.Itoa(id), "")
		}

		// Dequeue empty
		resp = doRequest("POST", "/jobs/dequeue", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Dequeue empty status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var dequeueResp struct {
			Job     interface{} `json:"job"`
			Message string      `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&dequeueResp)

		if dequeueResp.Job != nil {
			t.Error("Dequeue empty should return nil job")
		}
		if dequeueResp.Message != "queue empty" {
			t.Errorf("Message = %q, want %q", dequeueResp.Message, "queue empty")
		}
	})

	t.Run("health endpoint", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/health", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Health request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Health status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	_ = strings.TrimSpace // suppress unused warning
}
