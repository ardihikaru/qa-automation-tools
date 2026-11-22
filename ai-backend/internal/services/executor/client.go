// Package executor is the client for playwright-executor, the systemd service
// on app-host (default http://10.8.0.9:8141) that runs every Playwright job.
//
// The backend submits runs and polls for results; the executor never calls
// the backend. Auth is a shared bearer token (PLAYWRIGHT_EXECUTOR_TOKEN).
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Run statuses reported by the executor.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// ErrRunNotFound is wrapped by the error of a request the executor answered with 404 (the run
// is unknown: it never existed or the executor restarted and lost it).
var ErrRunNotFound = errors.New("playwright-executor: run not found")

// ErrQueueFull is returned when the executor answers 429.
var ErrQueueFull = errors.New("playwright-executor queue is full")

// Config configures the client. An empty URL means "not configured".
type Config struct {
	URL     string
	Token   string
	Timeout time.Duration // per HTTP request; default 15s
}

// Event is one progress line of a run (CASE_AUTOMATION.md section 4): N is a per-run counter
// that grows by one; the executor keeps the last 500 and redacts and clips the text.
type Event struct {
	N       int    `json:"n"`
	At      string `json:"at,omitempty"`
	Level   string `json:"level,omitempty"` // info | warn | error
	Step    string `json:"step,omitempty"`
	Message string `json:"message"`
}

// Run mirrors the executor's RunRecord.
type Run struct {
	ID          string                 `json:"id"`
	JobID       string                 `json:"job_id"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	CreatedAt   string                 `json:"created_at,omitempty"`
	StartedAt   string                 `json:"started_at,omitempty"`
	CompletedAt string                 `json:"completed_at,omitempty"`
	DurationMs  int64                  `json:"duration_ms,omitempty"`
	// Events holds only the events newer than the `since` the request carried (an old executor
	// sends none).
	Events []Event `json:"events,omitempty"`
}

// Terminal reports whether the run has finished.
func (r *Run) Terminal() bool { return r.Status == StatusCompleted || r.Status == StatusFailed }

// SubmitRequest is the body of POST /v1/runs.
type SubmitRequest struct {
	JobID  string                 `json:"job_id"`
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// Client talks to playwright-executor.
type Client struct {
	cfg  Config
	http *http.Client

	healthMu     sync.Mutex
	healthAt     time.Time
	healthStatus string
}

// New returns a client, or nil when cfg.URL is empty.
func New(cfg Config) *Client {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Submit enqueues a run and returns it (status "queued").
func (c *Client) Submit(ctx context.Context, req SubmitRequest) (*Run, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var run Run
	if err := c.do(ctx, http.MethodPost, "/v1/runs", body, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// Get fetches a run by id.
func (c *Client) Get(ctx context.Context, id string) (*Run, error) {
	var run Run
	if err := c.do(ctx, http.MethodGet, "/v1/runs/"+id, nil, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// GetSince fetches a run with only the events whose n is greater than since.
func (c *Client) GetSince(ctx context.Context, id string, since int) (*Run, error) {
	var run Run
	path := "/v1/runs/" + id
	if since > 0 {
		path += "?since=" + strconv.Itoa(since)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// Wait polls a run until it is terminal, ctx is done, or polling keeps failing. It asks for
// new events only (`since`) and passes each update to onUpdate with just the events it has not
// shown yet; the final run carries the events of its last poll.
func (c *Client) Wait(ctx context.Context, id string, every time.Duration, onUpdate func(*Run)) (*Run, error) {
	failures := 0
	since := 0
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		run, err := c.GetSince(ctx, id, since)
		if err == nil {
			failures = 0
			fresh := run.Events[:0:0]
			for _, e := range run.Events {
				if e.N > since {
					fresh = append(fresh, e)
					since = e.N
				}
			}
			run.Events = fresh
			if onUpdate != nil {
				onUpdate(run)
			}
			if run.Terminal() {
				return run, nil
			}
		} else if failures++; failures >= 10 {
			return nil, fmt.Errorf("polling run %s: %w", id, err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("run %s did not finish: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

// Health returns "ok", "degraded" or "unreachable", cached for 30s.
func (c *Client) Health(ctx context.Context) string {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	if c.healthStatus != "" && time.Since(c.healthAt) < 30*time.Second {
		return c.healthStatus
	}
	status := "unreachable"
	hctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var h struct {
		Status string `json:"status"`
	}
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, c.cfg.URL+"/health", nil)
	if err == nil {
		if resp, err := c.http.Do(req); err == nil {
			_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&h)
			resp.Body.Close()
			if h.Status != "" {
				status = h.Status
			}
		}
	}
	c.healthStatus, c.healthAt = status, time.Now()
	return status
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out interface{}) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.URL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("playwright-executor %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrQueueFull
	case resp.StatusCode >= 300:
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("playwright-executor %s %s: HTTP 404: %s: %w", method, path, e.Error, ErrRunNotFound)
		}
		return fmt.Errorf("playwright-executor %s %s: HTTP %d: %s", method, path, resp.StatusCode, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
