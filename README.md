# agent-queue

A simple web-based job queue API backed by SQLite, built for agent workflows.

## What is this?

`agent-queue` is a lightweight job queue with an HTTP API. It stores jobs in a local SQLite database, making it easy to run without external infrastructure like Redis. Designed for agents to enqueue and process tasks with built-in deduplication.

## Why?

Sometimes you just need a queue. You don't need Redis, a message broker, or a hosted service. `agent-queue` gives you a simple, persistent, web-accessible queue that runs anywhere SQLite runs.

## Getting Started

```bash
go build -o agent-queue
```

## CLI

```bash
# Run the API server
agent-queue serve

# Enqueue a job
agent-queue enqueue --dedupe-key "task-123" --body "Process this"

# Dequeue a job (claim next pending)
agent-queue dequeue

# Extend a job claim
agent-queue extend <job-id>

# Delete a job
agent-queue delete <job-id>

# List jobs
agent-queue list --status pending --page 1

# Run a command with job body as stdin
agent-queue run -- cat
agent-queue run -- ./process.sh
```

By default, the CLI connects to `http://localhost:8080`. Set the `AGENT_QUEUE_URL` environment variable to change this:

```bash
export AGENT_QUEUE_URL=http://your-server:8080
export AGENT_QUEUE_API_KEY=your-secret-key
```

The CLI uses the same `X-API-Key` header and `AGENT_QUEUE_API_KEY` environment variable as the server.

## Environment Variables

| Variable               | Default           | Description                              |
|------------------------|-------------------|------------------------------------------|
| `AGENT_QUEUE_API_KEY`  | (required)       | API key for authentication              |
| `AGENT_QUEUE_DB_PATH`  | `agent-queue.db` | Path to the SQLite database file         |
| `AGENT_QUEUE_URL`      | `http://localhost:8080` | Server URL (CLI only)           |
| `AGENT_QUEUE_CLAIM_TIMEOUT` | `5m`         | Time before a claim expires (e.g., `30s`, `5m`, `1h`) |
| `AGENT_QUEUE_MAX_RETRIES`  | `3`            | Times a job can timeout before being marked 'failed' |

## Authentication

All requests require an API key via the `X-API-Key` header:

```
X-API-Key: your-secret-key
```

The API key is configured via the `AGENT_QUEUE_API_KEY` environment variable. If not set, requests will be rejected with `401 Unauthorized`.

## API

### Enqueue

Add a new job to the queue.

```
POST /jobs
```

Request body:

```json
{
  "dedupe_key": "unique-id-123",
  "tags": ["scrape", "priority"],
  "body": "What to process"
}
```

| Field       | Required | Description                                      |
|-------------|----------|--------------------------------------------------|
| `dedupe_key` | Yes      | Unique key to prevent duplicate jobs             |
| `tags`      | No       | Array of tags for categorization                 |
| `body`      | Yes      | Text content to process                          |

Response:

```json
{
  "id": 1,
  "dedupe_key": "unique-id-123",
  "tags": ["scrape", "priority"],
  "body": "What to process",
  "status": "pending",
  "created_at": "2026-08-01T21:00:00Z"
}
```

If a job with the same `dedupe_key` already exists, returns the existing job without creating a duplicate.

If a job times out more than `AGENT_QUEUE_MAX_RETRIES` times, it is automatically marked as `failed`.

### Dequeue

Claim the next pending job (for workers).

```
POST /jobs/dequeue
```

Response (job claimed):

```json
{
  "id": 1,
  "dedupe_key": "unique-id-123",
  "tags": ["scrape", "priority"],
  "body": "What to process",
  "status": "processing",
  "created_at": "2026-08-01T21:00:00Z",
  "claimed_at": "2026-08-01T21:05:00Z"
}
```

Response (queue empty):

```json
{
  "job": null,
  "message": "queue empty"
}
```

Workers should periodically call `extend` while processing to prevent the claim from timing out. If a claim times out, the job is automatically released back to the pending queue.

### Extend

Extend the claim on a job (prevents timeout).

```
POST /jobs/:id/extend
```

Response:

```json
{
  "id": 1,
  "claimed_at": "2026-08-01T21:10:00Z"
}
```

### Delete

Remove a job from the queue.

```
DELETE /jobs/:id
```

Response: `204 No Content`

### List

Paginated list of jobs for review.

```
GET /jobs
```

Query params:

| Param   | Description                                      |
|---------|--------------------------------------------------|
| `page`  | Page number (default: 1)                         |
| `limit` | Items per page (default: 20, max: 100)           |
| `status`| Filter by status: `pending`, `processing`, `completed`, `failed` |
| `tag`   | Filter by tag (can be repeated)                   |

Response:

```json
{
  "jobs": [...],
  "page": 1,
  "limit": 20,
  "total": 150,
  "total_pages": 8
}
```

## Data Model

Jobs are stored in the `items` table:

| Column        | Type     | Description                              |
|---------------|----------|------------------------------------------|
| `id`          | INTEGER  | Auto-incrementing primary key           |
| `dedupe_key`  | TEXT     | Unique key for deduplication (indexed)   |
| `body`        | TEXT     | Job content                             |
| `tags`        | TEXT     | Comma-separated tags (JSON array)       |
| `status`      | TEXT     | Current state: `pending`, `processing`, `completed`, `failed` |
| `created_at`   | DATETIME | When the job was enqueued               |
| `claimed_at`   | DATETIME | When a worker claimed the job           |
| `retry_count`  | INTEGER  | Number of times the job timed out       |
| `processed_at` | DATETIME | When the job finished                   |

## Roadmap

- [x] HTTP API (enqueue, dequeue, extend, delete, list)
- [x] API key authentication
- [x] CLI tool (server + client commands)
- [x] Claim timeout and auto-release
