package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/nalanj/agent-queue/db"
)

type Handler struct {
	db     *db.DB
	apiKey string
}

func New(database *db.DB, apiKey string) *Handler {
	return &Handler{db: database, apiKey: apiKey}
}

func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" || key != h.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Auth check (skip for health endpoint)
	if r.URL.Path != "/health" {
		key := r.Header.Get("X-API-Key")
		if key == "" || key != h.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	path := r.URL.Path
	method := r.Method

	// Routes
	switch {
	case path == "/health" && method == "GET":
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
		return

	case path == "/jobs" && method == "POST":
		h.handleEnqueue(w, r)
		return

	case path == "/jobs" && method == "GET":
		h.handleList(w, r)
		return

	case path == "/jobs/dequeue" && method == "POST":
		h.handleDequeue(w, r)
		return

	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/extend") && method == "POST":
		idStr := strings.TrimPrefix(strings.TrimSuffix(path, "/extend"), "/jobs/")
		h.handleExtend(w, r, idStr)
		return

	case strings.HasPrefix(path, "/jobs/") && method == "DELETE":
		idStr := strings.TrimPrefix(path, "/jobs/")
		h.handleDelete(w, r, idStr)
		return

	case strings.HasPrefix(path, "/jobs/") && method == "GET":
		idStr := strings.TrimPrefix(path, "/jobs/")
		h.handleGet(w, r, idStr)
		return

	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

type EnqueueRequest struct {
	DedupeKey string   `json:"dedupe_key"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
}

func (h *Handler) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	var req EnqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.DedupeKey == "" {
		http.Error(w, "dedupe_key is required", http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}

	// Check if this is a dedupe hit by fetching existing
	existing, _ := h.db.GetJobByDedupeKey(req.DedupeKey)
	isNew := existing == nil

	job, err := h.db.CreateJob(req.DedupeKey, req.Body, req.Tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return 200 if dedupe hit, 201 if new
	status := http.StatusCreated
	if !isNew {
		status = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(job)
}

type DequeueResponse struct {
	Job     *db.Job `json:"job"`
	Message string  `json:"message,omitempty"`
}

func (h *Handler) handleDequeue(w http.ResponseWriter, r *http.Request) {
	job, err := h.db.ClaimJob()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := DequeueResponse{}
	if job == nil {
		resp.Message = "queue empty"
	} else {
		resp.Job = job
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleExtend(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	job, err := h.db.ExtendJob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteJob(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type ListResponse struct {
	Jobs       []db.Job `json:"jobs"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	Total      int      `json:"total"`
	TotalPages int      `json:"total_pages"`
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	status := r.URL.Query().Get("status")
	tags := r.URL.Query()["tag"]

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	jobs, total, err := h.db.ListJobs(page, limit, status, tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := total / limit
	if total%limit != 0 {
		totalPages++
	}

	resp := ListResponse{
		Jobs:       jobs,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	job, err := h.db.GetJob(id)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}
