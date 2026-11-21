package gaossync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// HISTORY_ANSWER.md: POST .../inbox/{externalId}/answer.

func TestAnswerInboxItem(t *testing.T) {
	ans := InboxAnswer{Choice: 3, Text: "rename it", Actor: InboxActor{Username: "ardi", Display: "ai-service as ardi"}}
	body := `{"externalId":"qa-job-1-pr","status":"answered","choice":3,"optionLabel":"Needs changes","text":"rename it","answeredAt":"2026-11-21T10:00:00Z","source":{"kind":"qa-tool","actor":{"username":"ardi"}}}`
	for _, status := range []int{200, 409, 410} {
		var calls int
		s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/products/p-6/inbox/qa-job-1-pr/answer" || r.Header.Get("Authorization") != "Bearer gaos_x" {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			var got map[string]any
			_ = json.NewDecoder(r.Body).Decode(&got)
			if got["choice"] != float64(3) || got["text"] != "rename it" || got["actor"].(map[string]any)["display"] != "ai-service as ardi" {
				t.Errorf("body %v", got)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		})
		res, err := s.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", ans)
		if err != nil || res.Already != (status == 409) || res.Expired != (status == 410) || res.Item.OptionLabel != "Needs changes" || !strings.Contains(string(res.Item.Source), "qa-tool") || calls != 1 {
			t.Fatalf("status %d: %+v %v calls=%d", status, res, err, calls)
		}
	}
}

func TestAnswerInboxItemFailures(t *testing.T) {
	ans := InboxAnswer{Choice: 1, Actor: InboxActor{Username: "ardi"}}
	for _, c := range []struct {
		status int
		body   string
		note   string
	}{
		{404, `secret-body`, "no answer endpoint"}, {422, `{"error":"bad-choice"}`, "bad-choice"}, {422, `{"error":"bad-text"}`, "bad-text"},
		{400, `{"error":"bad-actor"}`, "bad-actor"}, {400, `{"error":"Some <b>secret-body</b>"}`, "refused"}, {401, ``, "rejected"},
		{500, `secret-body`, "HTTP 500"}, {200, `{"externalId":"other"}`, "unexpected body"}, {409, `nope`, "unexpected body"}, {410, `nope`, "unexpected body"},
	} {
		var calls int
		s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		})
		_, err := s.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", ans)
		var f *Failure
		if err == nil || !errors.As(err, &f) || f.Status != c.status || !strings.Contains(err.Error(), c.note) ||
			strings.Contains(err.Error(), "secret-body") || strings.Contains(err.Error(), "http://") || calls != 1 {
			t.Errorf("%d: %v calls=%d", c.status, err, calls)
		}
	}
	// 429: one retry after Retry-After, then a Failure.
	var n int
	s429 := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	var slept time.Duration
	s429.cl.sleep = func(_ context.Context, d time.Duration) error { slept = d; return nil }
	if _, err := s429.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", ans); err == nil || n != 2 || slept != MaxAnswerRetryAfter {
		t.Errorf("429: %v calls=%d slept=%v", err, n, slept)
	}
	n = 0
	sOK := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"externalId":"qa-job-1-pr","status":"answered","choice":1,"optionLabel":"Approve"}`))
	})
	if res, err := sOK.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", ans); err != nil || res.Already || n != 2 {
		t.Errorf("429 then 200: %v %v calls=%d", res, err, n)
	}
	for _, u := range []string{"", "operator", "bad name", strings.Repeat("a", 65)} {
		if _, err := sOK.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", InboxAnswer{Choice: 1, Actor: InboxActor{Username: u}}); err == nil {
			t.Errorf("username %q accepted", u)
		}
	}
	// Timeout: a transport-class failure (status 0) within the caller's budget.
	s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := s.AnswerInboxItem(ctx, "p-6", "qa-job-1-pr", ans)
	var f *Failure
	if !errors.As(err, &f) || f.Status != 0 {
		t.Errorf("timeout: %v", err)
	}
	if _, err := s.AnswerInboxItem(context.Background(), "p-6", "bad id", ans); err == nil {
		t.Error("bad external id accepted")
	}
	if _, err := s.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", InboxAnswer{Choice: 0, Actor: InboxActor{Username: "a"}}); err == nil {
		t.Error("choice 0 accepted")
	}
}
