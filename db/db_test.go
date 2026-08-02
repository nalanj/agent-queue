package db

import (
	"os"
	"testing"
)

func TestNew(t *testing.T) {
	tmp := t.TempDir() + "/test.db"
	defer os.Remove(tmp)

	db, err := New(tmp)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
}

func TestJob_CRUD(t *testing.T) {
	tmp := t.TempDir() + "/test.db"
	defer os.Remove(tmp)

	db, err := New(tmp)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Create a job
	job, err := db.CreateJob("dedupe-1", "test body", []string{"tag1"})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID == 0 {
		t.Error("CreateJob() job.ID should not be 0")
	}
	if job.DedupeKey != "dedupe-1" {
		t.Errorf("CreateJob() job.DedupeKey = %q, want %q", job.DedupeKey, "dedupe-1")
	}
	if job.Body != "test body" {
		t.Errorf("CreateJob() job.Body = %q, want %q", job.Body, "test body")
	}
	if job.Status != "pending" {
		t.Errorf("CreateJob() job.Status = %q, want %q", job.Status, "pending")
	}

	// Deduplication: create same dedupe key should return existing job
	existing, err := db.CreateJob("dedupe-1", "different body", []string{"tag2"})
	if err != nil {
		t.Fatalf("CreateJob() duplicate error = %v", err)
	}
	if existing.ID != job.ID {
		t.Errorf("CreateJob() duplicate should return same ID, got %d, want %d", existing.ID, job.ID)
	}

	// Get job by ID
	fetched, err := db.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if fetched.Body != "test body" {
		t.Errorf("GetJob() fetched.Body = %q, want %q", fetched.Body, "test body")
	}

	// Claim job
	claimed, err := db.ClaimJob()
	if err != nil {
		t.Fatalf("ClaimJob() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimJob() should return a job")
	}
	if claimed.Status != "processing" {
		t.Errorf("ClaimJob() job.Status = %q, want %q", claimed.Status, "processing")
	}
	if claimed.ClaimedAt == nil {
		t.Error("ClaimJob() job.ClaimedAt should not be nil")
	}

	// Extend job claim
	extended, err := db.ExtendJob(claimed.ID)
	if err != nil {
		t.Fatalf("ExtendJob() error = %v", err)
	}
	if extended.ClaimedAt.Equal(*claimed.ClaimedAt) {
		t.Error("ExtendJob() should update claimed_at")
	}

	// Delete job
	if err := db.DeleteJob(claimed.ID); err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}

	// Verify deleted
	_, err = db.GetJob(claimed.ID)
	if err == nil {
		t.Error("GetJob() after delete should return error")
	}
}

func TestListJobs(t *testing.T) {
	tmp := t.TempDir() + "/test.db"
	defer os.Remove(tmp)

	db, err := New(tmp)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Create jobs with different statuses and tags
	db.CreateJob("dedupe-1", "job 1", []string{"a"})
	db.CreateJob("dedupe-2", "job 2", []string{"b"})
	db.CreateJob("dedupe-3", "job 3", []string{"a", "b"})

	// Claim one job
	db.ClaimJob()

	// List all
	jobs, total, err := db.ListJobs(1, 10, "", nil)
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if total != 3 {
		t.Errorf("ListJobs() total = %d, want %d", total, 3)
	}
	if len(jobs) != 3 {
		t.Errorf("ListJobs() len = %d, want %d", len(jobs), 3)
	}

	// Filter by status
	jobs, _, err = db.ListJobs(1, 10, "processing", nil)
	if err != nil {
		t.Fatalf("ListJobs(status) error = %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("ListJobs(status) len = %d, want %d", len(jobs), 1)
	}

	// Filter by tag
	jobs, _, err = db.ListJobs(1, 10, "", []string{"a"})
	if err != nil {
		t.Fatalf("ListJobs(tag) error = %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("ListJobs(tag) len = %d, want %d", len(jobs), 2)
	}

	// Pagination
	jobs, _, err = db.ListJobs(1, 2, "", nil)
	if err != nil {
		t.Fatalf("ListJobs(paginate) error = %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("ListJobs(paginate) len = %d, want %d", len(jobs), 2)
	}
}
