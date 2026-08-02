package db

import (
	"database/sql"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Job struct {
	ID         int64      `json:"id"`
	DedupeKey  string     `json:"dedupe_key"`
	Body       string     `json:"body"`
	Tags       []string   `json:"tags"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ClaimedAt  *time.Time `json:"claimed_at,omitempty"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}

func NewFromEnv() (*DB, error) {
	path := os.Getenv("AGENT_QUEUE_DB_PATH")
	if path == "" {
		path = "agent-queue.db"
	}
	return New(path)
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dedupe_key TEXT NOT NULL UNIQUE,
			body TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			claimed_at DATETIME,
			processed_at DATETIME
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_items_dedupe_key ON items(dedupe_key)`)
	if err != nil {
		return err
	}

	_, err = db.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_items_status ON items(status)`)
	return err
}

func (db *DB) CreateJob(dedupeKey, body string, tags []string) (*Job, error) {
	// Check for existing job with same dedupe_key
	existing, err := db.GetJobByDedupeKey(dedupeKey)
	if err == nil {
		return existing, nil // dedupe hit
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Insert new job
	tagsJSON := "[]"
	if len(tags) > 0 {
		tagsJSON = `["` + strings.Join(tags, `","`) + `"]`
	}

	result, err := db.conn.Exec(
		`INSERT INTO items (dedupe_key, body, tags, status) VALUES (?, ?, ?, 'pending')`,
		dedupeKey, body, tagsJSON,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetJob(id)
}

func (db *DB) GetJob(id int64) (*Job, error) {
	row := db.conn.QueryRow(
		`SELECT id, dedupe_key, body, tags, status, created_at, claimed_at, processed_at FROM items WHERE id = ?`,
		id,
	)

	var job Job
	var tagsStr string
	var claimedAt, processedAt sql.NullTime

	err := row.Scan(&job.ID, &job.DedupeKey, &job.Body, &tagsStr, &job.Status, &job.CreatedAt, &claimedAt, &processedAt)
	if err != nil {
		return nil, err
	}

	job.Tags = parseTags(tagsStr)
	if claimedAt.Valid {
		job.ClaimedAt = &claimedAt.Time
	}
	if processedAt.Valid {
		job.ProcessedAt = &processedAt.Time
	}

	return &job, nil
}

func (db *DB) GetJobByDedupeKey(dedupeKey string) (*Job, error) {
	row := db.conn.QueryRow(
		`SELECT id, dedupe_key, body, tags, status, created_at, claimed_at, processed_at FROM items WHERE dedupe_key = ?`,
		dedupeKey,
	)

	var job Job
	var tagsStr string
	var claimedAt, processedAt sql.NullTime

	err := row.Scan(&job.ID, &job.DedupeKey, &job.Body, &tagsStr, &job.Status, &job.CreatedAt, &claimedAt, &processedAt)
	if err != nil {
		return nil, err
	}

	job.Tags = parseTags(tagsStr)
	if claimedAt.Valid {
		job.ClaimedAt = &claimedAt.Time
	}
	if processedAt.Valid {
		job.ProcessedAt = &processedAt.Time
	}

	return &job, nil
}

func (db *DB) ClaimJob() (*Job, error) {
	now := time.Now()

	// Find oldest pending job and claim it
	row := db.conn.QueryRow(
		`SELECT id FROM items WHERE status = 'pending' ORDER BY created_at ASC LIMIT 1`,
	)

	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // queue empty
		}
		return nil, err
	}

	_, err := db.conn.Exec(
		`UPDATE items SET status = 'processing', claimed_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return nil, err
	}

	return db.GetJob(id)
}

func (db *DB) ExtendJob(id int64) (*Job, error) {
	now := time.Now()

	_, err := db.conn.Exec(
		`UPDATE items SET claimed_at = ? WHERE id = ? AND status = 'processing'`,
		now, id,
	)
	if err != nil {
		return nil, err
	}

	return db.GetJob(id)
}

func (db *DB) DeleteJob(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM items WHERE id = ?`, id)
	return err
}

type ListOptions struct {
	Page   int
	Limit  int
	Status string
	Tags   []string
}

func (db *DB) ListJobs(page, limit int, status string, tags []string) ([]Job, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Build query
	query := `SELECT id, dedupe_key, body, tags, status, created_at, claimed_at, processed_at FROM items WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM items WHERE 1=1`
	args := []interface{}{}

	if status != "" {
		query += ` AND status = ?`
		countQuery += ` AND status = ?`
		args = append(args, status)
	}

	if len(tags) > 0 {
		for _, tag := range tags {
			query += ` AND tags LIKE ?`
			countQuery += ` AND tags LIKE ?`
			args = append(args, "%"+tag+"%")
		}
	}

	// Get total count
	var total int
	countArgs := args
	err := db.conn.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Add pagination
	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var tagsStr string
		var claimedAt, processedAt sql.NullTime

		err := rows.Scan(&job.ID, &job.DedupeKey, &job.Body, &tagsStr, &job.Status, &job.CreatedAt, &claimedAt, &processedAt)
		if err != nil {
			return nil, 0, err
		}

		job.Tags = parseTags(tagsStr)
		if claimedAt.Valid {
			job.ClaimedAt = &claimedAt.Time
		}
		if processedAt.Valid {
			job.ProcessedAt = &processedAt.Time
		}

		jobs = append(jobs, job)
	}

	return jobs, total, nil
}

func parseTags(tagsJSON string) []string {
	tagsJSON = strings.TrimSpace(tagsJSON)
	if tagsJSON == "" || tagsJSON == "[]" {
		return []string{}
	}

	// Simple parse: ["tag1","tag2"]
	tagsJSON = strings.TrimPrefix(tagsJSON, "[")
	tagsJSON = strings.TrimSuffix(tagsJSON, "]")
	tagsJSON = strings.ReplaceAll(tagsJSON, `"`, "")

	parts := strings.Split(tagsJSON, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if parts[0] == "" {
		return []string{}
	}
	return parts
}
