package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nalanj/agent-queue/db"
)

func setupTestServer(t *testing.T, jobs map[string]*db.Job) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth
		if r.Header.Get("X-API-Key") != "test-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/jobs":
			if r.Method == "POST" {
				var req struct {
					DedupeKey string   `json:"dedupe_key"`
					Body      string   `json:"body"`
					Tags      []string `json:"tags"`
				}
				json.NewDecoder(r.Body).Decode(&req)

				if existing, ok := jobs[req.DedupeKey]; ok {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(existing)
					return
				}

				job := &db.Job{
					ID:        int64(len(jobs) + 1),
					DedupeKey: req.DedupeKey,
					Body:      req.Body,
					Tags:      req.Tags,
					Status:    "pending",
				}
				jobs[req.DedupeKey] = job

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(job)
			} else if r.Method == "GET" {
				var jobList []db.Job
				for _, j := range jobs {
					jobList = append(jobList, *j)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(struct {
					Jobs       []db.Job `json:"jobs"`
					Page       int      `json:"page"`
					Limit      int      `json:"limit"`
					Total      int      `json:"total"`
					TotalPages int      `json:"total_pages"`
				}{
					Jobs:       jobList,
					Page:       1,
					Limit:      20,
					Total:      len(jobList),
					TotalPages: 1,
				})
			}

		case "/jobs/dequeue":
			var pending *db.Job
			for _, j := range jobs {
				if j.Status == "pending" {
					j.Status = "processing"
					pending = j
					break
				}
			}
			w.Header().Set("Content-Type", "application/json")
			if pending == nil {
				json.NewEncoder(w).Encode(struct {
					Job     *db.Job `json:"job"`
					Message string  `json:"message"`
				}{Message: "queue empty"})
			} else {
				json.NewEncoder(w).Encode(struct {
					Job     *db.Job `json:"job"`
					Message string  `json:"message"`
				}{Job: pending})
			}

		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
}

func TestClient_Enqueue(t *testing.T) {
	jobs := make(map[string]*db.Job)
	server := setupTestServer(t, jobs)
	defer server.Close()

	c := New()
	c.BaseURL = server.URL
	c.APIKey = "test-key"

	job, err := c.Enqueue("key-1", "test body", []string{"tag1"})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if job.DedupeKey != "key-1" {
		t.Errorf("DedupeKey = %q, want %q", job.DedupeKey, "key-1")
	}
	if job.Body != "test body" {
		t.Errorf("Body = %q, want %q", job.Body, "test body")
	}

	// Dedupe
	existing, err := c.Enqueue("key-1", "different body", nil)
	if err != nil {
		t.Fatalf("Enqueue() dedupe error = %v", err)
	}
	if existing.ID != job.ID {
		t.Errorf("Dedupe should return same ID")
	}
}

func TestClient_Dequeue(t *testing.T) {
	jobs := map[string]*db.Job{
		"key-1": {ID: 1, DedupeKey: "key-1", Body: "job 1", Status: "pending"},
	}
	server := setupTestServer(t, jobs)
	defer server.Close()

	c := New()
	c.BaseURL = server.URL
	c.APIKey = "test-key"

	job, err := c.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}

	if job.Status != "processing" {
		t.Errorf("Status = %q, want %q", job.Status, "processing")
	}
}

func TestClient_List(t *testing.T) {
	jobs := map[string]*db.Job{
		"key-1": {ID: 1, DedupeKey: "key-1", Body: "job 1", Status: "pending"},
		"key-2": {ID: 2, DedupeKey: "key-2", Body: "job 2", Status: "pending"},
	}
	server := setupTestServer(t, jobs)
	defer server.Close()

	c := New()
	c.BaseURL = server.URL
	c.APIKey = "test-key"

	result, err := c.List(1, 20, "", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want %d", result.Total, 2)
	}
}
