package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/objectstore"
)

type fakeObj struct {
	data []byte
	mod  time.Time
}

type fakeStore struct {
	mu       sync.Mutex
	objs     map[string]fakeObj
	now      func() time.Time
	getErr   error
	statErr  error
	putErr   error
	putGate  map[string]chan struct{} // Put of this key blocks until the channel is closed
	log      []string
	modShift time.Duration // added to LastModified on write (simulates skew)
}

func newFakeStore(now func() time.Time) *fakeStore {
	return &fakeStore{objs: map[string]fakeObj{}, now: now, putGate: map[string]chan struct{}{}}
}

func (s *fakeStore) rec(op, key string) {
	s.log = append(s.log, op+" "+key)
}

func (s *fakeStore) Stat(_ context.Context, key string) (objectstore.Info, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec("stat", key)
	if s.statErr != nil {
		return objectstore.Info{}, false, s.statErr
	}
	o, ok := s.objs[key]
	return objectstore.Info{Size: int64(len(o.data)), LastModified: o.mod}, ok, nil
}

func (s *fakeStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec("get", key)
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	o, ok := s.objs[key]
	return o.data, ok, nil
}

func (s *fakeStore) Put(_ context.Context, key, _ string, data []byte) error {
	s.mu.Lock()
	gate := s.putGate[key]
	s.mu.Unlock()
	if gate != nil {
		<-gate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec("put", key)
	if s.putErr != nil {
		return s.putErr
	}
	s.objs[key] = fakeObj{data: append([]byte(nil), data...), mod: s.now().Add(s.modShift)}
	return nil
}

func (s *fakeStore) count(op string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, l := range s.log {
		if strings.HasPrefix(l, op+" ") {
			n++
		}
	}
	return n
}

func (s *fakeStore) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.objs {
		out = append(out, k)
	}
	return out
}

type fakeSrc struct {
	mu   sync.Mutex
	info KeyInfo
	snap *ChecklistSnapshot
	err  error
}

func (f *fakeSrc) ChecklistKeyInfo(context.Context, int, bool) (KeyInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info, f.err
}

func (f *fakeSrc) ChecklistSnapshot(context.Context, int, bool) (*ChecklistSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	c := *f.snap
	return &c, nil
}

type cacheEnv struct {
	c       *ExportCache
	store   *fakeStore
	src     *fakeSrc
	renders atomic.Int32
	clock   atomic.Int64  // unix nanoseconds
	gate    chan struct{} // when non-nil, render blocks until it is closed
}

func newCacheEnv(t *testing.T) *cacheEnv {
	t.Helper()
	e := &cacheEnv{}
	e.clock.Store(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC).UnixNano())
	now := func() time.Time { return time.Unix(0, e.clock.Load()).UTC() }
	e.store = newFakeStore(now)
	e.src = &fakeSrc{
		info: KeyInfo{SyncID: 5, ProductName: "SUT", HistoryMaxID: 9},
		snap: &ChecklistSnapshot{ProductName: "SUT", SyncID: 5, HistoryMaxID: 9},
	}
	e.c = NewExportCache(e.src, e.store, "dev-test", time.Second)
	e.c.now = now
	e.c.render = func(s *ChecklistSnapshot, format string, upd bool) ([]byte, string, error) {
		e.renders.Add(1)
		if e.gate != nil {
			<-e.gate
		}
		return []byte(fmt.Sprintf("%s|sync%d|%v", format, s.SyncID, upd)), "ct/" + format, nil
	}
	return e
}

func (e *cacheEnv) advance(d time.Duration) { e.clock.Add(int64(d)) }

func mustServe(t *testing.T, e *cacheEnv, q ExportQuery) ExportResult {
	t.Helper()
	r, err := e.c.Serve(context.Background(), 1, q)
	if err != nil {
		t.Fatalf("serve %+v: %v", q, err)
	}
	return r
}

func TestExportMissThenHit(t *testing.T) {
	e := newCacheEnv(t)
	q := ExportQuery{Format: "pdf"}
	r := mustServe(t, e, q)
	if r.Cache != CacheMiss || string(r.Body) != "pdf|sync5|false" || r.Filename != "SUT-MODULE-CHECKLIST.pdf" {
		t.Fatalf("first: %+v", r)
	}
	e.c.Wait()
	if e.renders.Load() != 3 {
		t.Fatalf("renders = %d, want one per format", e.renders.Load())
	}
	if len(e.store.keys()) != 3 {
		t.Fatalf("stored %v", e.store.keys())
	}
	r = mustServe(t, e, q)
	if r.Cache != CacheHit || e.renders.Load() != 3 {
		t.Fatalf("second: %+v renders=%d", r, e.renders.Load())
	}
	// The other formats were stored with the first generation: hits, no rendering.
	for _, f := range []string{"xlsx", "csv"} {
		if r := mustServe(t, e, ExportQuery{Format: f}); r.Cache != CacheHit || !strings.HasPrefix(string(r.Body), f+"|") {
			t.Errorf("%s: %+v", f, r)
		}
	}
	if e.renders.Load() != 3 {
		t.Errorf("renders = %d", e.renders.Load())
	}
}

func TestExportKeyChanges(t *testing.T) {
	e := newCacheEnv(t)
	base := e.c.exportDir(1, 5, "SUT", false, 0)
	if !strings.HasPrefix(base, "qa-automation/dev-test/knowledge-exports/1/modules-checklist/sync-5/v1/n") || !strings.HasSuffix(base, "/plain") {
		t.Fatalf("key shape: %s", base)
	}
	if !strings.Contains(base, fmt.Sprintf("/v%d/", TemplateVersion)) {
		t.Error("key lacks the template version")
	}
	other := NewExportCache(e.src, e.store, "prod", time.Second)
	variants := map[string]string{
		"sync id":     e.c.exportDir(1, 6, "SUT", false, 0),
		"product":     e.c.exportDir(2, 5, "SUT", false, 0),
		"renamed":     e.c.exportDir(1, 5, "SUT renamed", false, 0),
		"last-update": e.c.exportDir(1, 5, "SUT", true, 3),
		"watermark":   e.c.exportDir(1, 5, "SUT", true, 4),
		"namespace":   other.exportDir(1, 5, "SUT", false, 0),
	}
	seen := map[string]string{base: "base"}
	for n, k := range variants {
		if prev, dup := seen[k]; dup {
			t.Errorf("%s gives the same key as %s", n, prev)
		}
		seen[k] = n
	}
	// The plain variant ignores the watermark.
	if e.c.exportDir(1, 5, "SUT", false, 99) != base {
		t.Error("plain key depends on the history watermark")
	}
	// End to end: a new sync id and a rename each cause a new generation.
	mustServe(t, e, ExportQuery{Format: "csv"})
	e.c.Wait()
	e.src.mu.Lock()
	e.src.info.SyncID, e.src.snap.SyncID = 6, 6
	e.src.mu.Unlock()
	if r := mustServe(t, e, ExportQuery{Format: "csv"}); r.Cache != CacheMiss {
		t.Errorf("new sync: %+v", r)
	}
	e.c.Wait()
	e.src.mu.Lock()
	e.src.info.ProductName, e.src.snap.ProductName = "Renamed", "Renamed"
	e.src.mu.Unlock()
	if r := mustServe(t, e, ExportQuery{Format: "csv"}); r.Cache != CacheMiss || r.Filename != "Renamed-MODULE-CHECKLIST.csv" {
		t.Errorf("rename: %+v", r)
	}
}

func TestExportForce(t *testing.T) {
	e := newCacheEnv(t)
	q := ExportQuery{Format: "xlsx"}
	mustServe(t, e, q)
	e.c.Wait()
	renders := e.renders.Load()

	// Within the cooldown the force is ignored.
	e.advance(10 * time.Second)
	r := mustServe(t, e, ExportQuery{Format: "xlsx", Force: true})
	if r.Cache != CacheHit || e.renders.Load() != renders {
		t.Fatalf("force inside cooldown: %+v", r)
	}
	// Past it, the object is regenerated and overwritten.
	e.advance(31 * time.Second)
	r = mustServe(t, e, ExportQuery{Format: "xlsx", Force: true})
	e.c.Wait()
	if r.Cache != CacheRegenerated || e.renders.Load() != renders+3 {
		t.Fatalf("force after cooldown: %+v renders=%d", r, e.renders.Load())
	}
	// A forced request for a missing object regenerates too.
	e2 := newCacheEnv(t)
	if r := mustServe(t, e2, ExportQuery{Format: "pdf", Force: true}); r.Cache != CacheRegenerated {
		t.Errorf("force on empty store: %+v", r)
	}
	e2.c.Wait()

	// Negative age (store clock ahead of ours) counts as 0: inside the cooldown.
	e3 := newCacheEnv(t)
	e3.store.modShift = time.Hour
	mustServe(t, e3, ExportQuery{Format: "csv"})
	e3.c.Wait()
	n := e3.renders.Load()
	e3.advance(time.Minute)
	if r := mustServe(t, e3, ExportQuery{Format: "csv", Force: true}); r.Cache != CacheHit || e3.renders.Load() != n {
		t.Errorf("negative age: %+v", r)
	}
}

func TestExportStoreFailuresBypass(t *testing.T) {
	boom := &objectstore.OpError{Op: "get", Code: "AccessDenied"}

	// Get fails: generate, stream, no Put at all.
	e := newCacheEnv(t)
	e.store.getErr = boom
	r := mustServe(t, e, ExportQuery{Format: "pdf"})
	e.c.Wait()
	if r.Cache != CacheBypass || string(r.Body) != "pdf|sync5|false" || e.store.count("put") != 0 {
		t.Fatalf("get error: %+v puts=%d", r, e.store.count("put"))
	}

	// Stat fails on a forced request: same.
	e = newCacheEnv(t)
	e.store.statErr = boom
	r = mustServe(t, e, ExportQuery{Format: "csv", Force: true})
	e.c.Wait()
	if r.Cache != CacheBypass || e.store.count("put") != 0 {
		t.Fatalf("stat error: %+v", r)
	}

	// Put of the requested object fails: bypass, the other two are skipped.
	e = newCacheEnv(t)
	e.store.putErr = &objectstore.OpError{Op: "put", Code: "InternalError"}
	r = mustServe(t, e, ExportQuery{Format: "xlsx"})
	e.c.Wait()
	if r.Cache != CacheBypass || string(r.Body) == "" || e.store.count("put") != 1 {
		t.Fatalf("put error: %+v puts=%d", r, e.store.count("put"))
	}
}

func TestExportExtraPutsRunAfterReturn(t *testing.T) {
	e := newCacheEnv(t)
	// Hold the two extra objects back: Serve must return without them.
	dir := e.c.exportDir(1, 5, "SUT", false, 0)
	gate := make(chan struct{})
	e.store.putGate[exportKey(dir, "xlsx")] = gate
	e.store.putGate[exportKey(dir, "csv")] = gate
	r := mustServe(t, e, ExportQuery{Format: "pdf"})
	if r.Cache != CacheMiss {
		t.Fatalf("%+v", r)
	}
	if got := e.store.keys(); len(got) != 1 || got[0] != exportKey(dir, "pdf") {
		t.Fatalf("before the extras: %v", got)
	}
	close(gate)
	e.c.Wait()
	if len(e.store.keys()) != 3 {
		t.Fatalf("after Wait: %v", e.store.keys())
	}
	// A failing extra Put is ignored.
	e = newCacheEnv(t)
	e.store.putGate[exportKey(e.c.exportDir(1, 5, "SUT", false, 0), "csv")] = func() chan struct{} { c := make(chan struct{}); close(c); return c }()
	mustServe(t, e, ExportQuery{Format: "pdf"})
	e.c.Wait()
}

func TestExportConcurrentRendersOnce(t *testing.T) {
	e := newCacheEnv(t)
	e.gate = make(chan struct{})
	var wg sync.WaitGroup
	results := make([]ExportResult, 12)
	formats := []string{"pdf", "xlsx", "csv"}
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := e.c.Serve(context.Background(), 1, ExportQuery{Format: formats[i%3]})
			if err != nil {
				t.Error(err)
			}
			results[i] = r
		}(i)
	}
	time.Sleep(100 * time.Millisecond) // let every caller reach the flight
	close(e.gate)
	wg.Wait()
	e.c.Wait()
	if e.renders.Load() != 3 {
		t.Fatalf("renders = %d: concurrent identical requests must generate once", e.renders.Load())
	}
	for i, r := range results {
		if !strings.HasPrefix(string(r.Body), formats[i%3]+"|") {
			t.Errorf("result %d: %+v", i, r)
		}
	}
	if len(e.store.keys()) != 3 {
		t.Errorf("stored %v", e.store.keys())
	}
}

func TestExportForcedDoesNotJoinUnforced(t *testing.T) {
	e := newCacheEnv(t)
	e.gate = make(chan struct{})
	var wg sync.WaitGroup
	var plain, forced ExportResult
	wg.Add(2)
	go func() { defer wg.Done(); plain, _ = e.c.Serve(context.Background(), 1, ExportQuery{Format: "csv"}) }()
	time.Sleep(50 * time.Millisecond)
	go func() {
		defer wg.Done()
		forced, _ = e.c.Serve(context.Background(), 1, ExportQuery{Format: "csv", Force: true})
	}()
	time.Sleep(100 * time.Millisecond)
	close(e.gate)
	wg.Wait()
	e.c.Wait()
	if plain.Cache != CacheMiss || forced.Cache != CacheRegenerated {
		t.Fatalf("plain=%q forced=%q", plain.Cache, forced.Cache)
	}
	if e.renders.Load() != 6 {
		t.Errorf("renders = %d, want two generations", e.renders.Load())
	}
}

func TestExportNewerSyncStoredUnderItsOwnKey(t *testing.T) {
	e := newCacheEnv(t)
	// A sync committed between the light read (sync 5) and the full read (sync 6).
	e.src.snap.SyncID = 6
	mustServe(t, e, ExportQuery{Format: "csv"})
	e.c.Wait()
	for _, k := range e.store.keys() {
		if !strings.Contains(k, "/sync-6/") {
			t.Errorf("object stored under the stale key: %s", k)
		}
	}
}

func TestExportErrors(t *testing.T) {
	e := newCacheEnv(t)
	e.src.err = ErrNoSucceededSync
	if _, err := e.c.Serve(context.Background(), 1, ExportQuery{Format: "pdf"}); !errors.Is(err, ErrNoSucceededSync) {
		t.Errorf("err = %v", err)
	}
	if e.store.count("get")+e.store.count("put") != 0 {
		t.Error("the store was used without a succeeded sync")
	}
	e.src.err = nil
	e.c.render = func(*ChecklistSnapshot, string, bool) ([]byte, string, error) {
		return nil, "", errors.New("render bug")
	}
	if _, err := e.c.Serve(context.Background(), 1, ExportQuery{Format: "pdf"}); err == nil || e.store.count("put") != 0 {
		t.Errorf("renderer error must fail the request and store nothing: %v", err)
	}
	if _, err := e.c.Serve(context.Background(), 1, ExportQuery{Format: "docx"}); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestExportBaseName(t *testing.T) {
	long := strings.Repeat("a", 70)
	cases := map[string]string{
		"SUT":           "SUT",
		"My Product / v2": "My-Product-v2",
		"  --x--  ":       "x",
		"日本語":             "product-7",
		`a"b;c/d`:         "a-b-c-d",
		"Prod\r\nname":    "Prod-name",
		long:              strings.Repeat("a", 60),
		"":                "product-7",
	}
	for in, want := range cases {
		if got := ExportBaseName(in, 7); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
