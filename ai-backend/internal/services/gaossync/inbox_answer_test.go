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
	for _, status := range []int{200, 409} {
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
		if err != nil || res.Already != (status == 409) || res.Item.OptionLabel != "Needs changes" || !strings.Contains(string(res.Item.Source), "qa-tool") || calls != 1 {
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
		{404, `secret-body`, "no answer endpoint"}, {410, ``, "expired"}, {422, ``, "refused the choice"}, {401, ``, "rejected"},
		{429, ``, "HTTP 429"}, {500, `secret-body`, "HTTP 500"}, {200, `{"externalId":"other"}`, "unexpected body"}, {409, `nope`, "unexpected body"},
	} {
		var calls int
		s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(c.status); _, _ = w.Write([]byte(c.body)) })
		_, err := s.AnswerInboxItem(context.Background(), "p-6", "qa-job-1-pr", ans)
		var f *Failure
		if err == nil || !errors.As(err, &f) || f.Status != c.status || !strings.Contains(err.Error(), c.note) ||
			strings.Contains(err.Error(), "secret-body") || strings.Contains(err.Error(), "http://") || calls != 1 {
			t.Errorf("%d: %v calls=%d", c.status, err, calls)
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
