package gaossync

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
	"time"
)

const (
	maxResponseBytes = 2 << 20
	maxRetries429    = 3
	maxRetryAfter    = 10 * time.Second
	sourceName       = "qa-automation-tools"
)

// Failure is an HTTP-level failure of a push: the push stops and is recorded as failed.
// Note is safe to store and show: it never holds the token, a URL or a response body.
type Failure struct {
	Status int // 0 for a transport or shape failure
	Note   string
}

func (f *Failure) Error() string { return f.Note }

type rejection struct {
	CaseID string `json:"caseId"`
	Reason string `json:"reason"`
}

// casesResponse and resultsResponse are the 200 answers. Required fields are pointers so a
// missing one is told from a zero; unknown fields are ignored so GAOS can add some.
type casesResponse struct {
	Created       *int            `json:"created"`
	Unchanged     *int            `json:"unchanged"`
	Rejected      *[]rejection    `json:"rejected"`
	EpicsAdded    int             `json:"epicsAdded"`
	CardsAdded    int             `json:"cardsAdded"`
	CardsDeferred int             `json:"cardsDeferred"`
	CatHead       json.RawMessage `json:"catHead"`
}

type resultsResponse struct {
	Applied   *int            `json:"applied"`
	Unchanged *int            `json:"unchanged"`
	Rejected  *[]rejection    `json:"rejected"`
	CatHead   json.RawMessage `json:"catHead"`
}

type client struct {
	base    string
	token   string
	hc      *http.Client
	timeout time.Duration
	sleep   func(ctx context.Context, d time.Duration) error
}

func newHTTPClient() *http.Client {
	return &http.Client{
		// A redirect is never followed: it would carry the bearer token to another place.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var hints = map[int]string{
	http.StatusUnauthorized:       "the token was rejected",
	http.StatusForbidden:          "the token is not allowed to write here",
	http.StatusNotFound:           "the GAOS product id is wrong or not enabled for this token",
	http.StatusServiceUnavailable: "GAOS is unavailable",
}

// post sends body as JSON to base+path and decodes a 200 answer into out (strictly, via check).
// 429 is retried after Retry-After; every other non-200 is a Failure. step names the request
// in failure notes ("cases batch 2/9").
func (c *client) post(ctx context.Context, path, step string, body, out any) *Failure {
	payload, err := json.Marshal(body)
	if err != nil {
		return &Failure{Note: "could not encode " + step}
	}
	for attempt := 0; ; attempt++ {
		status, retryAfter, data, f := c.once(ctx, path, payload)
		if f != nil {
			f.Note = step + ": " + f.Note
			return f
		}
		switch {
		case status == http.StatusOK:
			if err := json.Unmarshal(data, out); err != nil {
				return &Failure{Note: step + ": GAOS answered 200 with an unreadable body"}
			}
			return nil
		case status == http.StatusTooManyRequests:
			if attempt >= maxRetries429 {
				return &Failure{Status: status, Note: fmt.Sprintf("%s: GAOS answered HTTP 429 after %d retries (one write per token at a time)", step, maxRetries429)}
			}
			if err := c.sleep(ctx, retryAfter); err != nil {
				return &Failure{Note: step + ": stopped while waiting to retry (time budget)"}
			}
		default:
			note := fmt.Sprintf("%s: GAOS answered HTTP %d", step, status)
			if h, ok := hints[status]; ok {
				note += " (" + h + ")"
			}
			return &Failure{Status: status, Note: note}
		}
	}
}

// once makes one request. The Failure it returns covers transport errors and an oversized body.
func (c *client) once(ctx context.Context, path string, payload []byte) (status int, retryAfter time.Duration, data []byte, f *Failure) {
	rctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return 0, 0, nil, &Failure{Note: "could not build the request"}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		// The error text carries the URL; it is dropped, only the class is kept.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(rctx.Err(), context.DeadlineExceeded) {
			return 0, 0, nil, &Failure{Note: "no answer from GAOS in time"}
		}
		return 0, 0, nil, &Failure{Note: "could not reach GAOS"}
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return 0, 0, nil, &Failure{Note: "reading the GAOS answer failed"}
	}
	if len(data) > maxResponseBytes {
		return 0, 0, nil, &Failure{Note: "the GAOS answer is larger than 2 MiB"}
	}
	return resp.StatusCode, parseRetryAfter(resp.Header.Get("Retry-After")), data, nil
}

// parseRetryAfter reads delta-seconds; anything else (or less than 1) is 1 s; the cap is 10 s.
func parseRetryAfter(v string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return time.Second
	}
	d := time.Duration(n) * time.Second
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

func checkRejected(r *[]rejection, batch int) error {
	if r == nil {
		return errors.New("missing rejected")
	}
	if len(*r) > batch {
		return errors.New("more rejections than cases")
	}
	for _, x := range *r {
		if x.CaseID == "" || x.Reason == "" {
			return errors.New("rejection without caseId or reason")
		}
	}
	return nil
}

func (r *casesResponse) check(batch int) error {
	if r.Created == nil || r.Unchanged == nil {
		return errors.New("missing counters")
	}
	if *r.Created < 0 || *r.Unchanged < 0 || r.EpicsAdded < 0 || r.CardsAdded < 0 || r.CardsDeferred < 0 {
		return errors.New("negative counter")
	}
	return checkRejected(r.Rejected, batch)
}

func (r *resultsResponse) check(batch int) error {
	if r.Applied == nil || r.Unchanged == nil {
		return errors.New("missing counters")
	}
	if *r.Applied < 0 || *r.Unchanged < 0 {
		return errors.New("negative counter")
	}
	return checkRejected(r.Rejected, batch)
}
