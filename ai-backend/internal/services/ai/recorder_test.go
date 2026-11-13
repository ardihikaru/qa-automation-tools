package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

type fakeProvider struct {
	res ChatResult
	err error
}

func (f fakeProvider) Name() string                  { return ProviderAIManager }
func (f fakeProvider) Configured() bool              { return true }
func (f fakeProvider) Health(context.Context) string { return "ok" }
func (f fakeProvider) Upstream() string              { return "claude-v3" }
func (f fakeProvider) Chat(context.Context, []models.Message, ChatOpts) (ChatResult, error) {
	return f.res, f.err
}

type memRecorder struct{ recs []CallRecord }

func (m *memRecorder) RecordCall(r CallRecord) { m.recs = append(m.recs, r) }

func TestChatMessagesRecordsSuccess(t *testing.T) {
	rec := &memRecorder{}
	s := NewServiceWithProvider(fakeProvider{res: ChatResult{Text: "hi", Provider: "claude-v3", Model: "m", InputTokens: 3, OutputTokens: 2, CostUSD: 0.01, CostIDR: 160}}, time.Second)
	s.SetRecorder(rec)
	if _, err := s.ChatMessages(context.Background(), nil, ChatOpts{Feature: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(rec.recs))
	}
	r := rec.recs[0]
	if r.Status != "ok" || r.Feature != "x" || r.InputTokens != 3 || r.CostIDR != 160 {
		t.Fatalf("bad record: %+v", r)
	}
}

func TestChatMessagesRecordsFailureKeepingUsage(t *testing.T) {
	rec := &memRecorder{}
	s := NewServiceWithProvider(fakeProvider{res: ChatResult{Provider: "other", InputTokens: 5, CostUSD: 0.02, Fallback: true}, err: errors.New("pinned")}, time.Second)
	s.SetRecorder(rec)
	if _, err := s.ChatMessages(context.Background(), nil, ChatOpts{}); err == nil {
		t.Fatal("want error")
	}
	r := rec.recs[0]
	if r.Status != "error" || r.Error != "pinned" || r.InputTokens != 5 || !r.Fallback || r.Feature != "" {
		t.Fatalf("bad record: %+v", r)
	}
	if got := s.LastUsage(); got.InputTokens != 0 {
		t.Fatalf("lastUsage must not update on failure: %+v", got)
	}
}
