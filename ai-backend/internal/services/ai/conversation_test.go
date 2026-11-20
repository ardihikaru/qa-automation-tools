package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

type memStore struct {
	got   []Exchange
	err   error
	panic bool
}

func (m *memStore) SaveExchange(_ context.Context, ex Exchange) error {
	if m.panic {
		panic("boom")
	}
	m.got = append(m.got, ex)
	return m.err
}

func TestChatMessagesHandsTheExchangeToTheStore(t *testing.T) {
	st := &memStore{}
	s := NewServiceWithProvider(fakeProvider{res: ChatResult{Text: "reply", Provider: "claude-v3", Model: "opus"}}, time.Second)
	s.SetConversationStore(st)
	sent := []models.Message{{Role: "user", Content: "hi", Kind: "prompt"}}
	ref := &ConvRef{Kind: ConvJob, Ref: "job-1", ProductID: 3, Title: "TC-AUTH-001"}
	out, err := s.ChatMessages(context.Background(), sent, ChatOpts{Feature: "case-automation", Conversation: ref})
	if err != nil || out != "reply" {
		t.Fatalf("result changed: %q %v", out, err)
	}
	if len(st.got) != 1 {
		t.Fatalf("exchanges %d", len(st.got))
	}
	ex := st.got[0]
	if ex.Ref != *ref || ex.Reply != "reply" || ex.Model != "opus" || ex.Feature != "case-automation" || len(ex.Sent) != 1 || ex.Sent[0].Kind != "prompt" {
		t.Errorf("exchange %+v", ex)
	}

	// No reference: a conversation of its own, titled with the feature.
	if _, err := s.ChatMessages(context.Background(), sent, ChatOpts{Feature: "classify-issue"}); err != nil {
		t.Fatal(err)
	}
	if r := st.got[1].Ref; r.Kind != ConvCall || !strings.HasPrefix(r.Ref, "call-") || r.Title != "classify-issue" {
		t.Errorf("default ref %+v", r)
	}
}

func TestAFailedCallStoresTheErrorAndNoReply(t *testing.T) {
	st := &memStore{}
	s := NewServiceWithProvider(fakeProvider{res: ChatResult{Text: "partial"}, err: errors.New("upstream 502")}, time.Second)
	s.SetConversationStore(st)
	if _, err := s.ChatMessages(context.Background(), nil, ChatOpts{}); err == nil {
		t.Fatal("the error must reach the caller")
	}
	if len(st.got) != 1 || st.got[0].Reply != "" || st.got[0].Err == nil {
		t.Fatalf("exchange %+v", st.got)
	}
}

func TestAStoreNeverChangesTheCall(t *testing.T) {
	for name, st := range map[string]*memStore{"failing": {err: errors.New("db down")}, "panicking": {panic: true}} {
		s := NewServiceWithProvider(fakeProvider{res: ChatResult{Text: "ok"}}, time.Second)
		s.SetConversationStore(st)
		out, err := s.ChatMessages(context.Background(), nil, ChatOpts{})
		if out != "ok" || err != nil {
			t.Errorf("%s store changed the call: %q %v", name, out, err)
		}
	}
}

// Message.Kind must never reach a provider (AI_CONVERSATIONS.md 3.4).
func TestKindIsNotSentToProviders(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		_, _ = w.Write([]byte(`{"text":"x","provider":"claude-v3","choices":[{"message":{"role":"assistant","content":"x"}}]}`))
	}))
	defer srv.Close()
	msgs := []models.Message{{Role: "user", Content: "hello", Kind: "prompt"}}
	am := NewAIManagerProvider(AIManagerConfig{URL: srv.URL + "/", LoginID: "qa", SecretKey: "s", Provider: "claude-v3", Timeout: time.Second})
	_, _ = am.Chat(context.Background(), msgs, ChatOpts{})
	mm := newMiniMaxProvider("key", srv.URL, "m", "/chat", time.Second)
	_, _ = mm.Chat(context.Background(), msgs, ChatOpts{})
	if len(bodies) != 2 {
		t.Fatalf("requests %d", len(bodies))
	}
	for i, b := range bodies {
		if strings.Contains(b, `"kind"`) || strings.Contains(b, `"Kind"`) || !strings.Contains(b, `"hello"`) {
			t.Errorf("request %d: %s", i, b)
		}
	}
}
