package gaossync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/testdb"
)

// fakeGAOS records every request and answers from a script (or with an echo of "all created").
type fakeGAOS struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	reqs     []recorded
	respond  func(n int, r recorded) (status int, header map[string]string, body string)
	inflight int
	maxIn    int
	delay    time.Duration
}

type recorded struct {
	path, auth, ctype string
	body              map[string]any
}

func (r recorded) cases() []any {
	v, _ := r.body["cases"].([]any)
	return v
}
func (r recorded) items() []any {
	v, _ := r.body["items"].([]any)
	return v
}

func newFake(t *testing.T) *fakeGAOS {
	f := &fakeGAOS{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		rec := recorded{path: req.URL.Path, auth: req.Header.Get("Authorization"), ctype: req.Header.Get("Content-Type"), body: body}
		f.mu.Lock()
		f.reqs = append(f.reqs, rec)
		n := len(f.reqs)
		f.inflight++
		f.maxIn = max(f.maxIn, f.inflight)
		respond, delay := f.respond, f.delay
		f.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		status, header, out := http.StatusOK, map[string]string{}, ""
		if respond != nil {
			status, header, out = respond(n, rec)
		} else {
			out = defaultAnswer(rec)
		}
		f.mu.Lock()
		f.inflight--
		f.mu.Unlock()
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, out)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func defaultAnswer(r recorded) string {
	if strings.HasSuffix(r.path, "/qa/cases") {
		return fmt.Sprintf(`{"created":%d,"unchanged":0,"rejected":[],"epicsAdded":1,"cardsAdded":1,"cardsDeferred":"","catHead":1700}`, len(r.cases()))
	}
	return fmt.Sprintf(`{"applied":%d,"unchanged":0,"rejected":[],"catHead":"abc"}`, len(r.items()))
}

func (f *fakeGAOS) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

// newSyncer builds a Syncer on a scratch DB, pointed at the fake, with the retry sleep recorded.
func newSyncer(t *testing.T, f *fakeGAOS) (*Syncer, *db.DB, *[]time.Duration) {
	t.Helper()
	d := testdb.New(t)
	s := New(d, f.srv.URL+"/api/v1", goodToken)
	if !s.Configured() {
		t.Fatal("must be configured")
	}
	var slept []time.Duration
	var mu sync.Mutex
	s.cl.sleep = func(ctx context.Context, dur time.Duration) error {
		mu.Lock()
		slept = append(slept, dur)
		mu.Unlock()
		return ctx.Err()
	}
	return s, d, &slept
}

func exec(t *testing.T, d *db.DB, q string, args ...any) {
	t.Helper()
	if _, err := d.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func seedProduct(t *testing.T, d *db.DB, integrated bool) int {
	t.Helper()
	res, err := d.Exec(context.Background(), `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES ('SUT','https://github.com/o/r','https://s','sut','main')`)
	if err != nil {
		t.Fatal(err)
	}
	pid := int(res.LastInsertId())
	exec(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (?, ?, 'p-6')`, pid, integrated)
	return pid
}

func seedEpic(t *testing.T, d *db.DB, pid int, code, title string) {
	t.Helper()
	exec(t, d, `INSERT INTO kb_epics (product_id, code, title, service, content_hash) VALUES (?, ?, ?, 'sut-svc', 'h')`, pid, code, title)
}

func seedCase(t *testing.T, d *db.DB, pid int, code, epic, story, status string, specs ...string) {
	t.Helper()
	sj, _ := json.Marshal(append([]string{}, specs...))
	var st any
	if story != "" {
		st = story
	}
	exec(t, d, `INSERT INTO kb_test_cases (product_id, code, epic_code, story_code, scenario, polarity, level, priority, status, automated, spec_paths, content_hash)
VALUES (?, ?, ?, ?, 'Given G When W Then T', '+', 'API', 'P1', ?, ?, ?, 'h')`, pid, code, epic, st, status, status == "automated", string(sj))
}

func outcome(t *testing.T, d *db.DB, pid int) (status, note string, at *time.Time) {
	t.Helper()
	var a *time.Time
	if err := d.QueryRow(context.Background(), `SELECT gaos_last_status, gaos_last_note, gaos_last_sync_at FROM product_settings WHERE product_id = ?`, pid).Scan(&status, &note, &a); err != nil {
		t.Fatal(err)
	}
	return status, note, a
}

func startAndWait(t *testing.T, s *Syncer, pid int) Result {
	t.Helper()
	run, err := s.Start(pid)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.Done:
	case <-time.After(20 * time.Second):
		t.Fatal("push did not finish")
	}
	return run.Result()
}

func TestPushSendsMappedCasesThenResults(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "Authentication")
	exec(t, d, `INSERT INTO kb_features (product_id, code, module_code, content_hash) VALUES (?, 'A01.07', 'A01', 'h'), (?, 'A01.02', 'A01', 'h')`, pid, pid)
	exec(t, d, `INSERT INTO kb_story_features (product_id, story_code, feature_code) VALUES (?, 'US-AUTH-01', 'A01.07'), (?, 'US-AUTH-01', 'A01.02')`, pid, pid)
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "US-AUTH-01", "planned")
	seedCase(t, d, pid, "TC-AUTH-002", "AUTH", "", "automated", "tests/auth/login.spec.ts", "other.ts")
	seedCase(t, d, pid, "TC-AUTH-003", "AUTH", "", "blocked")
	seedCase(t, d, pid, "TC-AUTH-004", "AUTH", "", "manual-only")
	seedCase(t, d, pid, "TC-AUTH-005", "AUTH", "", "wont")
	seedCase(t, d, pid, "L-001", "AUTH", "", "planned")
	seedCase(t, d, pid, "TC-LOGIN-001", "AUTH", "", "planned")
	seedCase(t, d, pid, "TC-AUTH-006", "AUTH", "", "planned")
	exec(t, d, `UPDATE kb_test_cases SET removed_at = UTC_TIMESTAMP() WHERE code = 'TC-AUTH-006'`)

	res := startAndWait(t, s, pid)
	if res.Status != "ok" || res.Sent != 2 || res.Skipped != 5 || res.Created != 2 || res.ResultsApplied != 2 {
		t.Fatalf("%+v", res)
	}
	reqs := f.requests()
	if len(reqs) != 2 || reqs[0].path != "/api/v1/products/p-6/qa/cases" || reqs[1].path != "/api/v1/products/p-6/qa/results" {
		t.Fatalf("requests: %+v", reqs)
	}
	for _, r := range reqs {
		if r.auth != "Bearer "+goodToken || r.ctype != "application/json" || r.body["source"] != "qa-automation-tools" {
			t.Fatalf("headers/source: %+v", r)
		}
	}
	epics := reqs[0].body["epics"].([]any)
	e := epics[0].(map[string]any)
	if len(epics) != 1 || e["code"] != "AUTH" || e["title"] != "Authentication" || e["service"] != "sut-svc" || e["source"] != "qa-automation-tools" || e["module"] != "A01" {
		t.Fatalf("epics: %+v", epics)
	}
	cs := reqs[0].cases()
	c1, c2 := cs[0].(map[string]any), cs[1].(map[string]any)
	if c1["caseId"] != "TC-AUTH-001" || c1["module"] != "A01" || c1["userStory"] != "US-AUTH-01" || c1["decision"] != "planned" || c1["reason"] != "" ||
		c1["implemented"] != false || c1["spec"] != "" || fmt.Sprint(c1["features"]) != "[A01.02 A01.07]" || c1["pri"] != "P1" {
		t.Fatalf("case 1: %+v", c1)
	}
	if c2["caseId"] != "TC-AUTH-002" || c2["implemented"] != true || c2["spec"] != "tests/auth/login.spec.ts" || fmt.Sprint(c2["features"]) != "[]" {
		t.Fatalf("case 2: %+v", c2)
	}
	items := reqs[1].items()
	i2 := items[1].(map[string]any)
	if len(items) != 2 || i2["caseId"] != "TC-AUTH-002" || i2["implemented"] != true || i2["spec"] != "tests/auth/login.spec.ts" {
		t.Fatalf("results: %+v", items)
	}
	if _, has := items[0].(map[string]any)["result"]; has {
		t.Fatal("run results must never be sent")
	}
	if st, note, at := outcome(t, d, pid); st != "ok" || at == nil || !strings.HasPrefix(note, "sent 2 cases") {
		t.Fatalf("stored: %q %q %v", st, note, at)
	}
}

func TestPushBatchesAt200(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "BIG", "Big")
	for i := 1; i <= 401; i++ {
		seedCase(t, d, pid, fmt.Sprintf("TC-BIG-%03d", i), "BIG", "", "planned")
	}
	res := startAndWait(t, s, pid)
	if res.Status != "ok" {
		t.Fatalf("%+v", res)
	}
	var sizes []string
	for _, r := range f.requests() {
		if strings.HasSuffix(r.path, "/qa/cases") {
			sizes = append(sizes, fmt.Sprintf("c%d/e%d", len(r.cases()), len(r.body["epics"].([]any))))
		} else {
			sizes = append(sizes, fmt.Sprintf("r%d", len(r.items())))
		}
	}
	// TC-BIG-001..401 all match the code pattern, so 401 cases are sent: 200 + 200 + 1.
	if got := strings.Join(sizes, " "); got != "c200/e1 c200/e1 c1/e1 r200 r200 r1" {
		t.Fatalf("batches: %s", got)
	}
	if res.Batches != 6 || res.Sent != 401 {
		t.Fatalf("%+v", res)
	}
}

func TestExistsDiffersIsInformationalOtherRejectionsArePartial(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "Authentication")
	for _, c := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003"} {
		seedCase(t, d, pid, c, "AUTH", "", "planned")
	}
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/cases") {
			return 200, nil, `{"created":1,"unchanged":0,"rejected":[{"caseId":"TC-AUTH-002","reason":"exists-differs"},{"caseId":"TC-AUTH-003","reason":"bad-spec"}],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1700}`
		}
		return 200, nil, defaultAnswer(r)
	}
	res := startAndWait(t, s, pid)
	if res.Status != "partial" || res.ExistsDiffers != 1 || res.Rejected != 1 || len(res.RejectedSample) != 1 || res.RejectedSample[0].Reason != "bad-spec" {
		t.Fatalf("%+v", res)
	}
	// Results are sent for the created case and the exists-differs one, not for the refused one.
	var ids []string
	for _, it := range f.requests()[1].items() {
		ids = append(ids, it.(map[string]any)["caseId"].(string))
	}
	if strings.Join(ids, ",") != "TC-AUTH-001,TC-AUTH-002" {
		t.Fatalf("results for %v", ids)
	}

	// Only exists-differs: ok.
	f.mu.Lock()
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/cases") {
			return 200, nil, `{"created":0,"unchanged":2,"rejected":[{"caseId":"TC-AUTH-002","reason":"exists-differs"}],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1700}`
		}
		return 200, nil, defaultAnswer(r)
	}
	f.mu.Unlock()
	if res := startAndWait(t, s, pid); res.Status != "ok" || res.ExistsDiffers != 1 || res.Rejected != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	f := newFake(t)
	s, d, slept := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		switch n {
		case 1:
			return 429, map[string]string{"Retry-After": "2"}, `{"error":"busy"}`
		case 2:
			return 429, map[string]string{"Retry-After": "300"}, ``
		}
		return 200, nil, defaultAnswer(r)
	}
	res := startAndWait(t, s, pid)
	if res.Status != "ok" {
		t.Fatalf("%+v", res)
	}
	if len(*slept) != 2 || (*slept)[0] != 2*time.Second || (*slept)[1] != 10*time.Second {
		t.Fatalf("waits: %v (Retry-After is honoured and capped at 10 s)", *slept)
	}
	reqs := f.requests()
	if len(reqs) != 4 || reqs[0].path != reqs[1].path || reqs[1].path != reqs[2].path {
		t.Fatalf("the same batch must be retried: %d requests", len(reqs))
	}
}

func TestRetryOn429GivesUpAfterThree(t *testing.T) {
	f := newFake(t)
	s, d, slept := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	f.respond = func(int, recorded) (int, map[string]string, string) {
		return 429, map[string]string{"Retry-After": "1"}, ``
	}
	res := startAndWait(t, s, pid)
	if res.Status != "failed" || !strings.Contains(res.Note, "HTTP 429 after 3 retries") || len(f.requests()) != 4 || len(*slept) != 3 {
		t.Fatalf("%+v requests=%d slept=%v", res, len(f.requests()), *slept)
	}
}

func TestHTTPFailuresStopTheSlice(t *testing.T) {
	for _, code := range []int{401, 403, 404, 503, 500, 400} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			f := newFake(t)
			s, d, _ := newSyncer(t, f)
			pid := seedProduct(t, d, true)
			seedEpic(t, d, pid, "AUTH", "A")
			seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
			f.respond = func(int, recorded) (int, map[string]string, string) {
				return code, nil, `{"error":"secret detail ` + goodToken + `"}`
			}
			var logs bytes.Buffer
			log.SetOutput(&logs)
			defer log.SetOutput(io.Discard)
			res := startAndWait(t, s, pid)
			if res.Status != "failed" || len(f.requests()) != 1 || !strings.Contains(res.Note, fmt.Sprintf("HTTP %d", code)) || !strings.HasPrefix(res.Note, "cases batch 1/1:") {
				t.Fatalf("%+v", res)
			}
			st, note, at := outcome(t, d, pid)
			if st != "failed" || at == nil {
				t.Fatalf("stored %q %v", st, at)
			}
			for _, text := range []string{res.Note, note, logs.String()} {
				if strings.Contains(text, goodToken) || strings.Contains(text, "secret detail") {
					t.Fatalf("token or body leaked: %q", text)
				}
			}
		})
	}
}

func TestRedirectIsRefusedAndTokenNotForwarded(t *testing.T) {
	var hits int
	var gotAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer other.Close()
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	for _, code := range []int{301, 302, 307, 308} {
		f.mu.Lock()
		f.reqs = nil
		f.respond = func(int, recorded) (int, map[string]string, string) {
			return code, map[string]string{"Location": other.URL + "/x"}, ""
		}
		f.mu.Unlock()
		res := startAndWait(t, s, pid)
		if res.Status != "failed" || !strings.Contains(res.Note, fmt.Sprintf("HTTP %d", code)) {
			t.Fatalf("%d: %+v", code, res)
		}
	}
	if hits != 0 || gotAuth != "" {
		t.Fatalf("a redirect was followed: hits=%d auth=%q", hits, gotAuth)
	}
}

func TestStrictResponseShapeAndSizeCap(t *testing.T) {
	bodies := map[string]string{
		"missing counters": `{"rejected":[]}`,
		"missing rejected": `{"created":1,"unchanged":0}`,
		"null rejected":    `{"created":1,"unchanged":0,"rejected":null}`,
		"negative":         `{"created":-1,"unchanged":0,"rejected":[]}`,
		"float":            `{"created":1.5,"unchanged":0,"rejected":[]}`,
		"string counter":   `{"created":"1","unchanged":0,"rejected":[]}`,
		"rejection no id":  `{"created":0,"unchanged":0,"rejected":[{"reason":"x"}]}`,
		"too many rejects": `{"created":0,"unchanged":0,"rejected":[{"caseId":"a","reason":"x"},{"caseId":"b","reason":"y"}]}`,
		"not json":         `<html>`,
		"too large":        `{"created":1,"unchanged":0,"rejected":[],"pad":"` + strings.Repeat("x", 2<<20) + `"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			s, d, _ := newSyncer(t, f)
			pid := seedProduct(t, d, true)
			seedEpic(t, d, pid, "AUTH", "A")
			seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
			f.respond = func(int, recorded) (int, map[string]string, string) { return 200, nil, body }
			res := startAndWait(t, s, pid)
			if res.Status != "failed" || len(f.requests()) != 1 {
				t.Fatalf("%+v", res)
			}
		})
	}
	// Unknown extra fields are fine.
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/cases") {
			return 200, nil, `{"created":1,"unchanged":0,"rejected":[],"newField":{"a":1}}`
		}
		return 200, nil, `{"applied":1,"unchanged":0,"rejected":[],"x":true}`
	}
	if res := startAndWait(t, s, pid); res.Status != "ok" {
		t.Fatalf("%+v", res)
	}
}

func TestIdempotentResend(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	seedCase(t, d, pid, "TC-AUTH-002", "AUTH", "", "automated", "a.spec.ts")
	first := startAndWait(t, s, pid)
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/cases") {
			return 200, nil, `{"created":0,"unchanged":2,"rejected":[],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1700}`
		}
		return 200, nil, `{"applied":0,"unchanged":2,"rejected":[],"catHead":"abc"}`
	}
	second := startAndWait(t, s, pid)
	if first.Status != "ok" || second.Status != "ok" || second.Created != 0 || second.Unchanged != 2 {
		t.Fatalf("%+v / %+v", first, second)
	}
	reqs := f.requests()
	if len(reqs) != 4 || fmt.Sprint(reqs[0].body) != fmt.Sprint(reqs[2].body) || fmt.Sprint(reqs[1].body) != fmt.Sprint(reqs[3].body) {
		t.Fatal("the second push must send the same payloads")
	}
}

func TestNothingToSendIsOk(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "blocked")
	res := startAndWait(t, s, pid)
	if res.Status != "ok" || len(f.requests()) != 0 || !strings.HasPrefix(res.Note, "nothing to send") {
		t.Fatalf("%+v", res)
	}
}

func TestStartRefusals(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	off := seedProduct(t, d, false)
	if _, err := s.Start(off); err != ErrNotIntegrated {
		t.Fatalf("not integrated: %v", err)
	}
	if _, err := s.Start(9999); err != ErrNotIntegrated {
		t.Fatalf("no settings row: %v", err)
	}
	unconfigured := New(d, "", "")
	if unconfigured.Configured() {
		t.Fatal("must not be configured")
	}
	if _, err := unconfigured.Start(off); err != ErrNotConfigured {
		t.Fatalf("unconfigured: %v", err)
	}
	unconfigured.Trigger(off) // must be a no-op
	New(d, "http://example.com/api", goodToken).Trigger(off)
	if len(f.requests()) != 0 {
		t.Fatal("nothing may be sent")
	}
}

func TestOnePushPerProductAndCoalescedTrigger(t *testing.T) {
	f := newFake(t)
	f.delay = 150 * time.Millisecond
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	run, err := s.Start(pid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(pid); err != ErrBusy {
		t.Fatalf("second Start while running: %v", err)
	}
	// Triggers during the run coalesce into exactly one more pass.
	deadline := time.Now().Add(5 * time.Second)
	for len(f.requests()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		s.Trigger(pid)
	}
	time.Sleep(50 * time.Millisecond) // let the trigger goroutines mark dirty
	<-run.Done
	if got := len(f.requests()); got != 4 {
		t.Fatalf("want 2 passes x 2 requests, got %d", got)
	}
	f.mu.Lock()
	maxIn := f.maxIn
	f.mu.Unlock()
	if maxIn != 1 {
		t.Fatalf("requests overlapped: %d in flight", maxIn)
	}
	s.mu.Lock()
	left := len(s.running)
	s.mu.Unlock()
	if left != 0 {
		t.Fatal("the running entry must be removed")
	}
}

func TestPushesOfTwoProductsNeverOverlap(t *testing.T) {
	f := newFake(t)
	f.delay = 60 * time.Millisecond
	s, d, _ := newSyncer(t, f)
	var runs []*Run
	for i := 0; i < 2; i++ {
		res, err := d.Exec(context.Background(), `INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES (?, 'https://github.com/o/r', 'https://s', ?, 'main')`, fmt.Sprintf("P%d", i), fmt.Sprintf("o/p%d", i))
		if err != nil {
			t.Fatal(err)
		}
		pid := int(res.LastInsertId())
		exec(t, d, `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (?, 1, ?)`, pid, fmt.Sprintf("p-%d", i+1))
		seedEpic(t, d, pid, "AUTH", "A")
		seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
		run, err := s.Start(pid)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
	}
	for _, r := range runs {
		<-r.Done
		if r.Result().Status != "ok" {
			t.Fatalf("%+v", r.Result())
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.maxIn != 1 || len(f.reqs) != 4 {
		t.Fatalf("one write in flight per token: max %d, requests %d", f.maxIn, len(f.reqs))
	}
}

func TestFlagTurnedOffOrIdChangedMidPushStopsQuietly(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if n == 1 { // the admin corrects the id after the first request
			exec(t, d, `UPDATE product_settings SET gaos_product_id = 'p-7', gaos_last_status = '', gaos_last_note = '' WHERE product_id = ?`, pid)
		}
		return 200, nil, defaultAnswer(r)
	}
	res := startAndWait(t, s, pid)
	if res.Status != "" || len(f.requests()) != 1 {
		t.Fatalf("a push must stop before the next request: %+v requests=%d", res, len(f.requests()))
	}
	if st, note, at := outcome(t, d, pid); st != "" || note != "" || at != nil {
		t.Fatalf("a stale push must not store an outcome: %q %q %v", st, note, at)
	}
}

func TestRetryOnlyWhenLastOutcomeIsNotOK(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	exec(t, d, `UPDATE product_settings SET gaos_last_status = 'ok' WHERE product_id = ?`, pid)
	s.Retry(pid)
	time.Sleep(300 * time.Millisecond)
	if len(f.requests()) != 0 {
		t.Fatal("an ok product must not be pushed by Retry")
	}
	exec(t, d, `UPDATE product_settings SET gaos_last_status = 'failed' WHERE product_id = ?`, pid)
	s.Retry(pid)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, _, _ := outcome(t, d, pid); st == "ok" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a failed product must be pushed by Retry")
}

func TestTransportErrorNoteHasNoURL(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "A")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	f.srv.Close()
	res := startAndWait(t, s, pid)
	if res.Status != "failed" || !strings.Contains(res.Note, "could not reach GAOS") || strings.Contains(res.Note, "127.0.0.1") || strings.Contains(res.Note, goodToken) {
		t.Fatalf("%+v", res)
	}
}

func TestNoteIsClipped(t *testing.T) {
	long := strings.Repeat("é", 400)
	if got := clipNote(long); len([]rune(got)) != 300 || !strings.HasSuffix(got, "...") {
		t.Fatalf("%d", len([]rune(got)))
	}
}

// The exact bodies GAOS v0.37.1 serves (captured from the live API on 2026-11-14): cardsDeferred is a
// STRING and catHead an integer. The first live push failed with "unreadable body" because this decoded
// cardsDeferred as an int; every fake in this file had agreed with the bug.
func TestGaosResponsesAsServed(t *testing.T) {
	var cr casesResponse
	if err := json.Unmarshal([]byte(`{"created":0,"unchanged":0,"rejected":[],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1700}`), &cr); err != nil || cr.check(1) != nil {
		t.Fatalf("the cases response as served did not decode: %v %v", err, cr.check(1))
	}
	var rr resultsResponse
	if err := json.Unmarshal([]byte(`{"applied":0,"unchanged":0,"rejected":[],"catHead":1700}`), &rr); err != nil || rr.check(1) != nil {
		t.Fatalf("the results response as served did not decode: %v", err)
	}
	// A deferred card is a reason, and counts as the epics that got none.
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "Authentication")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/cases") {
			return 200, nil, `{"created":1,"unchanged":0,"rejected":[],"epicsAdded":1,"cardsAdded":0,"cardsDeferred":"no-workflow","catHead":1}`
		}
		return 200, nil, defaultAnswer(r)
	}
	if res := startAndWait(t, s, pid); res.Status != "ok" || res.CardsDeferred != 1 || res.CardsAdded != 0 {
		t.Fatalf("a deferred card: %+v, want ok with 1 deferred", res)
	}
}

func seedRun(t *testing.T, d *db.DB, pid int, code, result, at, ref string) {
	t.Helper()
	exec(t, d, `INSERT INTO kb_case_results (product_id, case_code, result, run_at, run_ref, source) VALUES (?,?,?,?,?, 'ci')`, pid, code, result, at, ref)
}

func resultItems(t *testing.T, f *fakeGAOS) map[string]map[string]any {
	t.Helper()
	reqs := f.requests()
	out := map[string]map[string]any{}
	for _, it := range reqs[len(reqs)-1].items() {
		m := it.(map[string]any)
		out[m["caseId"].(string)] = m
	}
	return out
}

func TestPushCarriesTheStoredRun(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "Authentication")
	for _, c := range []string{"TC-AUTH-001", "TC-AUTH-002", "TC-AUTH-003", "TC-AUTH-004", "TC-AUTH-005"} {
		seedCase(t, d, pid, c, "AUTH", "", "planned")
	}
	seedCase(t, d, pid, "TC-AUTH-006", "AUTH", "", "blocked") // skipped: no results item even with a stored run
	seedRun(t, d, pid, "TC-AUTH-001", "pass", "2026-11-14 10:00:00", "ci-77")
	seedRun(t, d, pid, "TC-AUTH-002", "fail", "2026-11-14 10:00:00", "run-é")                     // not ASCII
	seedRun(t, d, pid, "TC-AUTH-003", "skipped", "2026-11-14 10:00:00", strings.Repeat("x", 120)) // the longest GAOS takes
	seedRun(t, d, pid, "TC-AUTH-004", "pass", "2026-11-14 10:00:00", "")
	seedRun(t, d, pid, "TC-AUTH-006", "pass", "2026-11-14 10:00:00", "ci")

	if res := startAndWait(t, s, pid); res.Status != "ok" || res.ResultsApplied != 5 {
		t.Fatalf("%+v", res)
	}
	items := resultItems(t, f)
	if len(items) != 5 {
		t.Fatalf("items: %v", items)
	}
	const wantAt = float64(1794650400) // 2026-11-14T10:00:00Z in unix SECONDS
	a := items["TC-AUTH-001"]
	if a["result"] != "pass" || a["runAt"] != wantAt || a["runRef"] != "ci-77" {
		t.Fatalf("pass item: %v", a)
	}
	if i := items["TC-AUTH-002"]; i["result"] != "fail" || i["runAt"] != wantAt {
		t.Fatalf("fail item: %v", i)
	} else if _, has := i["runRef"]; has {
		t.Fatalf("a non-ASCII runRef must be omitted: %v", i)
	}
	if i := items["TC-AUTH-003"]; i["result"] != "skipped" || i["runAt"] != wantAt || i["runRef"] != strings.Repeat("x", 120) {
		t.Fatalf("skipped item: %v", i)
	}
	// kb_case_results.run_ref is VARCHAR(120), so an over-long ref cannot be stored; the guard is unit-tested.
	long := resultItem{CaseID: "x"}.withRun(storedRun{result: "pass", runAt: 5, runRef: strings.Repeat("x", 121)}, true)
	if long.Result != "pass" || long.RunAt != 5 || long.RunRef != "" {
		t.Fatalf("an over-long runRef must be dropped alone: %+v", long)
	}
	if bad := (resultItem{CaseID: "x"}).withRun(storedRun{result: "weird", runAt: 5}, true); bad.Result != "" || bad.RunAt != 0 {
		t.Fatalf("an unknown result sends no run fields: %+v", bad)
	}
	if _, has := items["TC-AUTH-004"]["runRef"]; has {
		t.Fatalf("an empty runRef must be omitted: %v", items["TC-AUTH-004"])
	}
	// No stored run: the fields are absent, not empty.
	for _, k := range []string{"result", "runAt", "runRef"} {
		if _, has := items["TC-AUTH-005"][k]; has {
			t.Fatalf("TC-AUTH-005 must not carry %s: %v", k, items["TC-AUTH-005"])
		}
	}
	// runAt travels as an integer on the wire.
	raw, _ := json.Marshal(resultItem{CaseID: "x"}.withRun(storedRun{result: "pass", runAt: 1794650400}, true))
	if !strings.Contains(string(raw), `"runAt":1794650400}`) {
		t.Fatalf("wire: %s", raw)
	}
}

func TestStaleIsInformationalOtherRejectionsArePartial(t *testing.T) {
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "Authentication")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	seedCase(t, d, pid, "TC-AUTH-002", "AUTH", "", "planned")
	seedRun(t, d, pid, "TC-AUTH-001", "pass", "2026-11-14 10:00:00", "a")
	seedRun(t, d, pid, "TC-AUTH-002", "pass", "2026-11-14 10:00:00", "b")
	answer := func(rej string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.respond = func(n int, r recorded) (int, map[string]string, string) {
			if strings.HasSuffix(r.path, "/qa/results") {
				return 200, nil, `{"applied":0,"unchanged":0,"rejected":[` + rej + `],"catHead":1700}`
			}
			return 200, nil, defaultAnswer(r)
		}
	}
	answer(`{"caseId":"TC-AUTH-001","reason":"stale"},{"caseId":"TC-AUTH-002","reason":"stale"}`)
	res := startAndWait(t, s, pid)
	if res.Status != "ok" || res.ResultsStale != 2 || res.Rejected != 0 || len(res.RejectedSample) != 0 || !strings.Contains(res.Note, "2 results older") {
		t.Fatalf("stale must be informational: %+v", res)
	}
	answer(`{"caseId":"TC-AUTH-001","reason":"stale"},{"caseId":"TC-AUTH-002","reason":"future"}`)
	res = startAndWait(t, s, pid)
	if res.Status != "partial" || res.ResultsStale != 1 || res.Rejected != 1 || res.RejectedSample[0].Reason != "future" {
		t.Fatalf("a real rejection: %+v", res)
	}
}

// The exact bodies GAOS serves today (captured live): the results answer with a catHead integer,
// and the stale one.
func TestGaosResultsBodiesAsServedToday(t *testing.T) {
	for name, body := range map[string]string{
		"cases":   `{"created":0,"unchanged":0,"rejected":[],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1700}`,
		"results": `{"applied":68,"unchanged":0,"rejected":[],"catHead":1700}`,
		"stale":   `{"applied":0,"unchanged":0,"rejected":[{"caseId":"TC-CHMGMT-010","reason":"stale"}],"catHead":1700}`,
	} {
		if name == "cases" {
			var cr casesResponse
			if err := json.Unmarshal([]byte(body), &cr); err != nil || cr.check(1) != nil || rawString(cr.CatHead, "") != "1700" {
				t.Fatalf("%s: %v %v", name, err, cr.check(1))
			}
			continue
		}
		var rr resultsResponse
		if err := json.Unmarshal([]byte(body), &rr); err != nil || rr.check(1) != nil || rawString(rr.CatHead, "") != "1700" {
			t.Fatalf("%s did not decode: %v %v", name, err, rr.check(1))
		}
	}
	var rr resultsResponse
	_ = json.Unmarshal([]byte(`{"applied":68,"unchanged":0,"rejected":[],"catHead":1700}`), &rr)
	if *rr.Applied != 68 {
		t.Fatalf("applied %d", *rr.Applied)
	}
	// And end to end: the stale body as served keeps the push ok.
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "AUTH", "Authentication")
	seedCase(t, d, pid, "TC-AUTH-001", "AUTH", "", "planned")
	seedRun(t, d, pid, "TC-AUTH-001", "pass", "2026-11-14 10:00:00", "a")
	f.respond = func(n int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/results") {
			return 200, nil, `{"applied":0,"unchanged":0,"rejected":[{"caseId":"TC-AUTH-001","reason":"stale"}],"catHead":1700}`
		}
		return 200, nil, `{"created":0,"unchanged":1,"rejected":[],"epicsAdded":0,"cardsAdded":0,"cardsDeferred":"","catHead":1700}`
	}
	if res := startAndWait(t, s, pid); res.Status != "ok" || res.ResultsStale != 1 || res.CatHead != "1700" {
		t.Fatalf("%+v", res)
	}
}
