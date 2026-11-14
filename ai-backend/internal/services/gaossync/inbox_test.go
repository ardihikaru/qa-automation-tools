package gaossync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func inboxSyncer(t *testing.T, h http.HandlerFunc) *Syncer {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s := &Syncer{configured: true}
	s.cl = &client{base: srv.URL + "/api/v1", token: "gaos_x", hc: newHTTPClient(), timeout: 2 * time.Second,
		sleep: func(context.Context, time.Duration) error { return nil }}
	return s
}

func TestCreateInboxItem(t *testing.T) {
	req := InboxCreate{ExternalID: "qa-job-1-pr", Title: "t", Question: "q", Context: "c", ExpiresInHours: 72,
		Options: []InboxOption{{Label: "Approve"}, {Label: "Reject"}, {Label: "Needs changes"}}}
	for _, status := range []int{200, 201} {
		s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/products/p-6/inbox" || r.Header.Get("Authorization") != "Bearer gaos_x" {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			var got InboxCreate
			_ = json.NewDecoder(r.Body).Decode(&got)
			if got.ExternalID != "qa-job-1-pr" || len(got.Options) != 3 || got.ExpiresInHours != 72 {
				t.Errorf("body %+v", got)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"id":7,"externalId":"qa-job-1-pr","status":"open","created":true}`))
		})
		out, err := s.CreateInboxItem(context.Background(), "p-6", req)
		if err != nil || out.Status != "open" {
			t.Fatalf("status %d: %v %v", status, out, err)
		}
	}
}

func TestCreateInboxItemFailures(t *testing.T) {
	req := InboxCreate{ExternalID: "qa-job-1-pr", Options: []InboxOption{{Label: "A"}}}
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, ``}, {"server", 500, `secret-body`}, {"other id", 201, `{"externalId":"x","status":"open"}`}, {"garbage", 201, `nope`},
	}
	for _, c := range cases {
		s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(c.status); _, _ = w.Write([]byte(c.body)) })
		_, err := s.CreateInboxItem(context.Background(), "p-6", req)
		if err == nil || strings.Contains(err.Error(), "secret-body") || strings.Contains(err.Error(), "http://") {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := s.CreateInboxItem(context.Background(), "../x", req); err == nil {
		t.Error("bad product id accepted")
	}
	if _, err := s.CreateInboxItem(context.Background(), "p-6", InboxCreate{ExternalID: "bad id"}); err == nil {
		t.Error("bad external id accepted")
	}
	var nilS *Syncer
	if _, err := nilS.GetInboxItem(context.Background(), "p-6", "a"); err == nil {
		t.Error("nil syncer")
	}
}

func TestGetInboxItem(t *testing.T) {
	s := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/products/p-6/inbox/qa-job-1-pr" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"externalId":"qa-job-1-pr","status":"answered","choice":1,"optionLabel":"Approve","text":"lgtm","answeredAt":"2026-11-14T10:00:00Z"}`))
	})
	it, err := s.GetInboxItem(context.Background(), "p-6", "qa-job-1-pr")
	if err != nil || it.Status != InboxAnswered || it.OptionLabel != "Approve" || it.Text != "lgtm" {
		t.Fatalf("%+v %v", it, err)
	}
	s404 := inboxSyncer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	if _, err := s404.GetInboxItem(context.Background(), "p-6", "qa-job-1-pr"); err == nil {
		t.Fatal("404 must be an error")
	}
}
