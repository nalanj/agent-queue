package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/nalanj/agent-queue/db"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func setupTestDB(t *testing.T) *db.DB {
	tmp := t.TempDir() + "/test.db"
	testDB, err := db.New(tmp)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	if err := testDB.Migrate(); err != nil {
		t.Fatalf("Failed to migrate test db: %v", err)
	}
	return testDB
}

func withAuth(req *http.Request, key string) {
	req.Header.Set("X-API-Key", key)
}

func TestAuthMiddleware(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	h := New(testDB, "secret-key")

	tests := []struct {
		name       string
		apiKey     string
		wantStatus int
	}{
		{"valid key", "secret-key", http.StatusOK},
		{"invalid key", "wrong-key", http.StatusUnauthorized},
		{"no key", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/jobs", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			w := httptest.NewRecorder()
			handler := h.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("AuthMiddleware() status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleEnqueue(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	h := New(testDB, "secret-key")

	t.Run("enqueue new job", func(t *testing.T) {
		body := `{"dedupe_key": "key-1", "body": "test job", "tags": ["a", "b"]}`
		req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("HandleEnqueue() status = %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
		}

		var resp db.Job
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Decode response: %v", err)
		}

		if resp.DedupeKey != "key-1" {
			t.Errorf("dedupe_key = %q, want %q", resp.DedupeKey, "key-1")
		}
		if resp.Body != "test job" {
			t.Errorf("body = %q, want %q", resp.Body, "test job")
		}
		if len(resp.Tags) != 2 {
			t.Errorf("tags len = %d, want %d", len(resp.Tags), 2)
		}
	})

	t.Run("dedupe returns existing", func(t *testing.T) {
		body := `{"dedupe_key": "key-1", "body": "different body"}`
		req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("HandleEnqueue() dedupe status = %d, want %d", w.Code, http.StatusOK)
		}

		var resp db.Job
		json.NewDecoder(w.Body).Decode(&resp)
		if resp.Body != "test job" {
			t.Errorf("dedupe should return existing job body = %q, want %q", resp.Body, "test job")
		}
	})

	t.Run("missing dedupe_key", func(t *testing.T) {
		body := `{"body": "test"}`
		req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("HandleEnqueue() status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestHandleDequeue(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	h := New(testDB, "secret-key")

	t.Run("dequeue returns job", func(t *testing.T) {
		// Create a pending job
		testDB.CreateJob("key-1", "test job", nil)

		req := httptest.NewRequest("POST", "/jobs/dequeue", nil)
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("HandleDequeue() status = %d, want %d", w.Code, http.StatusOK)
		}

		var resp DequeueResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Decode response: %v", err)
		}

		if resp.Job == nil {
			t.Fatal("HandleDequeue() job should not be nil")
		}
		if resp.Job.Status != "processing" {
			t.Errorf("status = %q, want %q", resp.Job.Status, "processing")
		}
	})

	t.Run("dequeue empty queue", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/jobs/dequeue", nil)
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("HandleDequeue() empty status = %d, want %d", w.Code, http.StatusOK)
		}

		var resp DequeueResponse
		json.NewDecoder(w.Body).Decode(&resp)

		if resp.Job != nil {
			t.Errorf("HandleDequeue() empty job should be nil")
		}
		if resp.Message != "queue empty" {
			t.Errorf("message = %q, want %q", resp.Message, "queue empty")
		}
	})
}

func TestHandleExtend(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	h := New(testDB, "secret-key")

	t.Run("extend claimed job", func(t *testing.T) {
		job, _ := testDB.CreateJob("key-1", "test", nil)
		testDB.ClaimJob()

		req := httptest.NewRequest("POST", "/jobs/"+strconv.FormatInt(job.ID, 10)+"/extend", nil)
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("HandleExtend() status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestHandleDelete(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	h := New(testDB, "secret-key")

	t.Run("delete job", func(t *testing.T) {
		job, _ := testDB.CreateJob("key-1", "test", nil)

		req := httptest.NewRequest("DELETE", "/jobs/"+strconv.FormatInt(job.ID, 10), nil)
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("HandleDelete() status = %d, want %d", w.Code, http.StatusNoContent)
		}

		// Verify deleted
		_, err := testDB.GetJob(job.ID)
		if err == nil {
			t.Error("Job should be deleted")
		}
	})
}

func TestHandleList(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	h := New(testDB, "secret-key")

	t.Run("list jobs", func(t *testing.T) {
		testDB.CreateJob("key-1", "job 1", []string{"a"})
		testDB.CreateJob("key-2", "job 2", []string{"b"})

		req := httptest.NewRequest("GET", "/jobs", nil)
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("HandleList() status = %d, want %d", w.Code, http.StatusOK)
		}

		var resp ListResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Decode response: %v", err)
		}

		if resp.Total != 2 {
			t.Errorf("total = %d, want %d", resp.Total, 2)
		}
		if len(resp.Jobs) != 2 {
			t.Errorf("jobs len = %d, want %d", len(resp.Jobs), 2)
		}
	})

	t.Run("list with pagination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/jobs?page=1&limit=1", nil)
		withAuth(req, "secret-key")

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var resp ListResponse
		json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Jobs) != 1 {
			t.Errorf("jobs len = %d, want %d", len(resp.Jobs), 1)
		}
		if resp.TotalPages != 2 {
			t.Errorf("total_pages = %d, want %d", resp.TotalPages, 2)
		}
	})
}
