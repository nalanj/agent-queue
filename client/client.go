package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/nalanj/agent-queue/db"
)

// ErrJobNotFound is returned when a job does not exist
var ErrJobNotFound = errors.New("job not found")

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func New() *Client {
	baseURL := os.Getenv("AGENT_QUEUE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	apiKey := os.Getenv("AGENT_QUEUE_API_KEY")
	if apiKey == "" {
		fmt.Println("Warning: AGENT_QUEUE_API_KEY not set")
	}

	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.HTTP.Do(req)
}

type EnqueueRequest struct {
	DedupeKey string   `json:"dedupe_key"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
}

func (c *Client) Enqueue(dedupeKey, body string, tags []string) (*db.Job, error) {
	req := EnqueueRequest{
		DedupeKey: dedupeKey,
		Body:      body,
		Tags:      tags,
	}

	resp, err := c.doRequest("POST", "/jobs", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("enqueue failed: %s", string(body))
	}

	var job db.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &job, nil
}

func (c *Client) Dequeue() (*db.Job, error) {
	resp, err := c.doRequest("POST", "/jobs/dequeue", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dequeue failed: %s", string(body))
	}

	var result struct {
		Job     *db.Job `json:"job"`
		Message string  `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Job == nil {
		fmt.Println(result.Message)
		return nil, nil
	}

	return result.Job, nil
}

func (c *Client) Extend(jobID int64) (*db.Job, error) {
	path := fmt.Sprintf("/jobs/%d/extend", jobID)
	resp, err := c.doRequest("POST", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrJobNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extend failed: %s", string(body))
	}

	var job db.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &job, nil
}

func (c *Client) Delete(jobID int64) error {
	path := fmt.Sprintf("/jobs/%d", jobID)
	resp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s", string(body))
	}

	return nil
}

type ListResponse struct {
	Jobs       []db.Job `json:"jobs"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	Total      int      `json:"total"`
	TotalPages int      `json:"total_pages"`
}

func (c *Client) List(page, limit int, status, tag string) (*ListResponse, error) {
	path := fmt.Sprintf("/jobs?page=%d&limit=%d", page, limit)
	if status != "" {
		path += "&status=" + status
	}
	if tag != "" {
		path += "&tag=" + tag
	}

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list failed: %s", string(body))
	}

	var result ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetJob(jobID int64) (*db.Job, error) {
	path := fmt.Sprintf("/jobs/%d", jobID)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("job not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get job failed: %s", string(body))
	}

	var job db.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &job, nil
}
