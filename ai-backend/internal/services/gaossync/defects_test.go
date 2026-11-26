package gaossync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseresults"
)

func TestDefectDerivation(t *testing.T) {
	for _, c := range []struct{ level, want string }{{"API", "be"}, {"GQL", "be"}, {"WEB", "e2e"}, {"E2E", "e2e"}, {"X", ""}} {
		if got := AutomationType(c.level); got != c.want {
			t.Errorf("AutomationType(%s) = %q, want %q", c.level, got, c.want)
		}
	}
	for spec, want := range map[string]string{
		"tests/api/graphql/sample-service/records/edit.spec.ts": "graphql",
		"tests/api/rest/sample-service/records.spec.ts":         "rest",
		"tests/web/sample-app/records.spec.ts":                  "",
		"other/x.spec.ts":                                       "",
	} {
		if got := BackendKind(spec); got != want {
			t.Errorf("BackendKind(%s) = %q, want %q", spec, got, want)
		}
	}
	if d := ServiceDir("tests/api/graphql/sample-service/records/edit.spec.ts"); d != "sample-service" {
		t.Errorf("ServiceDir = %q", d)
	}
	if d := ServiceDir("tests/web/sample-app/x.spec.ts"); d != "sample-app" {
		t.Errorf("ServiceDir web = %q", d)
	}
	if d := ServiceDir("tests/web/x.spec.ts"); d != "" {
		t.Errorf("a spec right under the root has no service dir: %q", d)
	}
	for url, want := range map[string]string{
		"https://github.com/acme-org/sample-service":     "acme-org/sample-service",
		"https://github.com/acme-org/sample-service.git": "acme-org/sample-service",
		"git@github.com:acme-org/sample-service.git":     "acme-org/sample-service",
		"not a url": "",
	} {
		if got := repoSlug(url); got != want {
			t.Errorf("repoSlug(%s) = %q, want %q", url, got, want)
		}
	}
	repos := map[string]string{"sample-app": "acme-org/sample-app", "sample-service": "acme-org/sample-service", "sample-billing": "acme-org/sample-billing"}
	mapping := map[string][]string{"sample-app": {"sample-service", "sample-billing", "missing"}}
	if got := targetsFor("tests/api/rest/sample-service/a.spec.ts", "be", repos, mapping); strings.Join(got, ",") != "acme-org/sample-service" {
		t.Errorf("be targets: %v", got)
	}
	if got := targetsFor("tests/web/sample-app/a.spec.ts", "e2e", repos, mapping); strings.Join(got, ",") != "acme-org/sample-app,acme-org/sample-service,acme-org/sample-billing" {
		t.Errorf("e2e targets: %v", got)
	}
	if got := targetsFor("tests/api/rest/unknown/a.spec.ts", "be", repos, mapping); got == nil || len(got) != 0 {
		t.Errorf("unresolved dir must give an empty, non-nil list: %#v", got)
	}
}

func TestOperationsOf(t *testing.T) {
	src := "const q = `mutation editSampleRecord($id: ID!) { edit(id: $id) { id } }`;\n" +
		"await request.get(`/api/v1/records/${id}?full=1`);\n" +
		"await request.post('https://svc.example.test/records/search');\n" +
		"headers.get('x-token');\n" +
		"query ListRecords { records { id } }\n" +
		"await request.delete('/a'); await request.put('/b'); await request.patch('/c');"
	got := operationsOf(src)
	want := []string{"editSampleRecord", "api/v1/records/_id", "records/search", "ListRecords", "a"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("operations = %v, want %v", got, want)
	}
	for _, o := range got {
		if !operationRe.MatchString(o) {
			t.Errorf("operation %q does not match GAOS's pattern", o)
		}
	}
	if got := operationsOf("nothing here"); got == nil || len(got) != 0 {
		t.Errorf("no operations must be an empty non-nil list: %#v", got)
	}
}

func TestOperationsFollowImportsInTheSameRepo(t *testing.T) {
	f := newFake(t)
	s, _, _ := newSyncer(t, f)
	files := map[string]string{
		"tests/api/rest/sample-service/a.spec.ts":  "import { helper } from './helpers';\nawait request.get('/own/route');",
		"tests/api/rest/sample-service/helpers.ts": "export const x = () => request.post('/from/helper');",
	}
	s.SetSpecReader(func(_ context.Context, _ int, rel string) ([]byte, error) {
		if v, ok := files[rel]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("no file")
	})
	got := s.operationsFor(context.Background(), 1, "tests/api/rest/sample-service/a.spec.ts", "")
	if strings.Join(got, "|") != "own/route|from/helper" {
		t.Errorf("operations = %v", got)
	}
}

// The automation repo imports its helpers through tsconfig "paths" aliases (found live: DEF-922's
// spec reaches the GraphQL operation only through "@api/..."), so a relative-only walk found none.
func TestOperationsFollowTsconfigAliases(t *testing.T) {
	f := newFake(t)
	s, _, _ := newSyncer(t, f)
	files := map[string]string{
		"tsconfig.json":                       "{\n // comment\n \"compilerOptions\": {\"paths\": {\"@api/*\": [\"./src/api/*\"], \"@fixtures\": [\"./src/fixtures/index.ts\"]}}}",
		"tests/api/graphql/svc/a.spec.ts":     "import { Kit } from '@api/graphql/svc/x.kit';\nimport { test } from '@fixtures';",
		"src/api/graphql/svc/x.kit.ts":        "import { Op } from './x.operations';",
		"src/api/graphql/svc/x.operations.ts": "const q = `mutation editThing($d: In!) { editThing(data: $d) { status } }`;",
		"src/fixtures/index.ts":               "await request.post('/from/fixtures');",
	}
	s.SetSpecReader(func(_ context.Context, _ int, rel string) ([]byte, error) {
		if v, ok := files[rel]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("no file")
	})
	got := s.operationsFor(context.Background(), 1, "tests/api/graphql/svc/a.spec.ts", "")
	if strings.Join(got, "|") != "editThing|from/fixtures" {
		t.Errorf("operations = %v", got)
	}
}

// A spec imports shared fixtures and utilities before its service's helpers; those must not spend the
// operation cap or the file budget (found live on DEF-922).
func TestOperationsPreferTheSpecsOwnService(t *testing.T) {
	f := newFake(t)
	s, _, _ := newSyncer(t, f)
	files := map[string]string{
		"tsconfig.json":                       `{"compilerOptions":{"paths":{"@api/*":["./src/api/*"],"@utils/*":["./src/utils/*"]}}}`,
		"tests/api/graphql/svc/a.spec.ts":     "import { u1 } from '@utils/u1';\nimport { u2 } from '@utils/u2';\nimport { u3 } from '@utils/u3';\nimport { k } from '@api/graphql/svc/k';",
		"src/api/graphql/svc/k.ts":            "import { O } from './k.operations';",
		"src/api/graphql/svc/k.operations.ts": "const q = `mutation theOperation($d: In!) { theOperation(data: $d) { status } }`;",
	}
	for _, n := range []string{"u1", "u2", "u3"} {
		files["src/utils/"+n+".ts"] = "await r.get('/noise/" + n + "a'); await r.get('/noise/" + n + "b'); await r.get('/noise/" + n + "c');"
	}
	s.SetSpecReader(func(_ context.Context, _ int, rel string) ([]byte, error) {
		if v, ok := files[rel]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("no file")
	})
	got := s.operationsFor(context.Background(), 1, "tests/api/graphql/svc/a.spec.ts", "")
	if len(got) == 0 || got[0] != "theOperation" {
		t.Errorf("the service's operation must come first: %v", got)
	}
}

// The operations whose words appear in the spec rank first (DEF-922: editMedicationDispense behind
// five unrelated names of the same service).
func TestOperationsAreRankedByTheSpecsWords(t *testing.T) {
	f := newFake(t)
	s, _, _ := newSyncer(t, f)
	op := func(n string) string {
		return "const q = `mutation " + n + "($d: In!) { " + n + "(data: $d) { status } }`;\n"
	}
	files := map[string]string{
		"tests/api/graphql/svc/a.spec.ts": "import { K } from './k';\nawait kit.editDispense(tx); // medication prescription",
		"tests/api/graphql/svc/k.ts":      "import { O } from './ops';",
		"tests/api/graphql/svc/ops.ts":    op("alphaOne") + op("alphaTwo") + op("alphaThree") + op("alphaFour") + op("alphaFive") + op("updateMedicationDispensePrescriptionStatusVerified") + op("editMedicationDispense"),
	}
	s.SetSpecReader(func(_ context.Context, _ int, rel string) ([]byte, error) {
		if v, ok := files[rel]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("no file")
	})
	got := s.operationsFor(context.Background(), 1, "tests/api/graphql/svc/a.spec.ts", "")
	if len(got) != 5 || got[0] != "editMedicationDispense" {
		t.Errorf("operations = %v; the one the spec names must be first of five", got)
	}
}

func TestFitDefectsSplitsByCountAndBytes(t *testing.T) {
	items := make([]defectItem, 120)
	for i := range items {
		items[i] = defectItem{CaseID: fmt.Sprintf("TC-SMP-%03d", i), Excerpt: strings.Repeat("x", 6000)}
	}
	n := len(fitDefects(items, 50))
	if n >= 50 {
		t.Errorf("120 items of 6 KiB cannot fit one 240 KiB body in 50 items, got %d", n)
	}
	b, _ := json.Marshal(items[:n])
	if len(b) > defectBodyMax+4096 {
		t.Errorf("batch of %d is %d bytes", n, len(b))
	}
	small := make([]defectItem, 120)
	if n := len(fitDefects(small, 50)); n != 50 {
		t.Errorf("small items: batch = %d, want 50", n)
	}
	if n := len(fitDefects(items, 0)); n != 1 {
		t.Errorf("a size of 0 must still take one: %d", n)
	}
}

// ---- the push, against a fake GAOS ----

const specGQL = "tests/api/graphql/sample-service/records/edit.spec.ts"

type defectRig struct {
	f   *fakeGAOS
	s   *Syncer
	d   *db.DB
	pid int
}

func newDefectRig(t *testing.T) *defectRig {
	t.Helper()
	f := newFake(t)
	s, d, _ := newSyncer(t, f)
	pid := seedProduct(t, d, true)
	seedEpic(t, d, pid, "SMP", "Sample")
	exec(t, d, `INSERT INTO product_repos (product_id, name, repo_url, root_dir) VALUES (?, 'sample-service', 'https://github.com/acme-org/sample-service', 'r1')`, pid)
	seedCase(t, d, pid, "TC-SMP-008", "SMP", "", "automated", specGQL)
	exec(t, d, `UPDATE kb_test_cases SET level = 'GQL' WHERE product_id = ? AND code = 'TC-SMP-008'`, pid)
	s.SetSpecReader(func(_ context.Context, _ int, rel string) ([]byte, error) {
		if rel == specGQL {
			return []byte("const q = `mutation editSampleRecord($id: ID!) { edit(id: $id) { id } }`"), nil
		}
		return nil, fmt.Errorf("no file")
	})
	return &defectRig{f: f, s: s, d: d, pid: pid}
}

func (r *defectRig) state(t *testing.T, code, kind, defectID, title, excerpt string) string {
	t.Helper()
	h := caseresults.DefectHash(code, caseresults.Defect{Kind: kind, DefectID: defectID, Title: title})
	exec(t, r.d, `INSERT INTO qa_defect_state (product_id, case_code, kind, defect_id, title, excerpt, run_ref, run_at, hash)
VALUES (?,?,?,?,?,?, 'run-1', '2026-11-25 10:00:00', ?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), defect_id=VALUES(defect_id), title=VALUES(title), excerpt=VALUES(excerpt), hash=VALUES(hash)`,
		r.pid, code, kind, defectID, title, excerpt, h)
	return h
}

func (r *defectRig) defectReqs() []recorded {
	var out []recorded
	for _, q := range r.f.requests() {
		if strings.HasSuffix(q.path, "/qa/defects") {
			out = append(out, q)
		}
	}
	return out
}

func answerDefects(body string) func(int, recorded) (int, map[string]string, string) {
	return func(_ int, r recorded) (int, map[string]string, string) {
		if strings.HasSuffix(r.path, "/qa/defects") {
			return 200, nil, body
		}
		return 200, nil, defaultAnswer(r)
	}
}

func TestPushSendsAnXFailItemWithTheExactShape(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":1,"unchanged":0,"rejected":[]}`)
	r.state(t, "TC-SMP-008", "xfail", "DEF-101", "a negative quantity is accepted and stored", "DEF-101 a negative quantity is accepted and stored")
	res := startAndWait(t, r.s, r.pid)
	if res.Status != "ok" {
		t.Fatalf("status %s: %s", res.Status, res.Note)
	}
	reqs := r.defectReqs()
	if len(reqs) != 1 || reqs[0].path != "/api/v1/products/p-6/qa/defects" || reqs[0].auth != "Bearer "+goodToken {
		t.Fatalf("requests: %+v", reqs)
	}
	b, _ := json.Marshal(reqs[0].body)
	want := `{"items":[{"automationType":"be","backend":"graphql","caseId":"TC-SMP-008","defectId":"DEF-101","excerpt":"DEF-101 a negative quantity is accepted and stored","kind":"xfail","level":"GQL","operations":["editSampleRecord"],"runAt":1795600800,"runRef":"run-1","spec":"tests/api/graphql/sample-service/records/edit.spec.ts","targetRepos":["acme-org/sample-service"],"title":"a negative quantity is accepted and stored"}],"source":"push"}`
	if string(b) != want {
		t.Errorf("body:\n got %s\nwant %s", b, want)
	}
}

func TestADefectIsSentOnlyOnChange(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":1,"unchanged":0,"rejected":[]}`)
	r.state(t, "TC-SMP-008", "xfail", "DEF-101", "t", "e")
	startAndWait(t, r.s, r.pid)
	startAndWait(t, r.s, r.pid) // nothing changed
	if n := len(r.defectReqs()); n != 1 {
		t.Fatalf("defect requests after an unchanged re-push: %d, want 1", n)
	}
	// Only the excerpt changes: still nothing to say.
	exec(t, r.d, `UPDATE qa_defect_state SET excerpt = 'other output' WHERE product_id = ?`, r.pid)
	startAndWait(t, r.s, r.pid)
	if n := len(r.defectReqs()); n != 1 {
		t.Fatalf("an excerpt change alone must not push: %d", n)
	}
	// The kind changes (the fix landed): pushed again.
	r.state(t, "TC-SMP-008", "xpass", "DEF-101", "t", "e")
	startAndWait(t, r.s, r.pid)
	reqs := r.defectReqs()
	if len(reqs) != 2 || !strings.Contains(fmt.Sprint(reqs[1].body["items"]), "xpass") {
		t.Fatalf("the xpass must be pushed: %+v", reqs)
	}
}

func TestDefectBatchesAreSplitAt50(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = func(_ int, q recorded) (int, map[string]string, string) {
		if strings.HasSuffix(q.path, "/qa/defects") {
			return 200, nil, fmt.Sprintf(`{"applied":%d,"unchanged":0,"rejected":[]}`, len(q.items()))
		}
		return 200, nil, defaultAnswer(q)
	}
	for i := 0; i < 120; i++ {
		code := fmt.Sprintf("TC-SMP-%03d", 100+i)
		seedCase(t, r.d, r.pid, code, "SMP", "", "automated", specGQL)
		r.state(t, code, "fail", "", fmt.Sprintf("failure %d", i), "boom")
	}
	startAndWait(t, r.s, r.pid)
	sizes := []int{}
	for _, q := range r.defectReqs() {
		sizes = append(sizes, len(q.items()))
	}
	if fmt.Sprint(sizes) != "[50 50 20]" {
		t.Errorf("batch sizes %v, want [50 50 20]", sizes)
	}
}

func TestAnOlderGAOSWithoutTheRouteIsToleratedAndLosesNothing(t *testing.T) {
	for _, code := range []int{404, 405} {
		r := newDefectRig(t)
		r.f.respond = func(_ int, q recorded) (int, map[string]string, string) {
			if strings.HasSuffix(q.path, "/qa/defects") {
				return code, nil, `{"error":"not found"}`
			}
			return 200, nil, defaultAnswer(q)
		}
		r.state(t, "TC-SMP-008", "xfail", "DEF-101", "t", "e")
		res := startAndWait(t, r.s, r.pid)
		if res.Status != "ok" {
			t.Fatalf("HTTP %d: the product must not be failed: %s %s", code, res.Status, res.Note)
		}
		var pushed *string
		var until *string
		_ = r.d.QueryRow(context.Background(), `SELECT pushed_hash FROM qa_defect_state WHERE product_id = ?`, r.pid).Scan(&pushed)
		if pushed != nil {
			t.Errorf("HTTP %d: the state was marked pushed", code)
		}
		if err := r.d.QueryRow(context.Background(), `SELECT CAST(backoff_until AS CHAR) FROM qa_defect_push WHERE product_id = ?`, r.pid).Scan(&until); err != nil || until == nil {
			t.Fatalf("HTTP %d: no backoff recorded: %v", code, err)
		}
		// During the backoff no second defect request is made; after it GAOS gets it.
		startAndWait(t, r.s, r.pid)
		if n := len(r.defectReqs()); n != 1 {
			t.Errorf("HTTP %d: %d defect requests during the backoff, want 1", code, n)
		}
		exec(t, r.d, `UPDATE qa_defect_push SET backoff_until = DATE_SUB(UTC_TIMESTAMP(), INTERVAL 1 MINUTE)`)
		r.f.respond = answerDefects(`{"applied":1,"unchanged":0,"rejected":[]}`)
		startAndWait(t, r.s, r.pid)
		_ = r.d.QueryRow(context.Background(), `SELECT pushed_hash FROM qa_defect_state WHERE product_id = ?`, r.pid).Scan(&pushed)
		if pushed == nil {
			t.Errorf("HTTP %d: the state was not delivered once GAOS supports the route", code)
		}
	}
}

func TestRejectedDefectsAreNotRetriedUntilTheyChange(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":0,"unchanged":0,"rejected":[{"caseId":"TC-SMP-008","reason":"unknown-case"}]}`)
	r.state(t, "TC-SMP-008", "xfail", "DEF-101", "t", "e")
	startAndWait(t, r.s, r.pid)
	startAndWait(t, r.s, r.pid)
	if n := len(r.defectReqs()); n != 1 {
		t.Fatalf("a rejected item was retried: %d requests", n)
	}
	r.state(t, "TC-SMP-008", "xfail", "DEF-101", "a changed title", "e")
	startAndWait(t, r.s, r.pid)
	if n := len(r.defectReqs()); n != 2 {
		t.Fatalf("a changed item must be sent again: %d requests", n)
	}
}

func TestA413HalvesTheBatch(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = func(_ int, q recorded) (int, map[string]string, string) {
		if strings.HasSuffix(q.path, "/qa/defects") {
			if len(q.items()) > 10 {
				return 413, nil, `{"error":"too-large"}`
			}
			return 200, nil, fmt.Sprintf(`{"applied":%d,"unchanged":0,"rejected":[]}`, len(q.items()))
		}
		return 200, nil, defaultAnswer(q)
	}
	for i := 0; i < 30; i++ {
		code := fmt.Sprintf("TC-SMP-%03d", 100+i)
		seedCase(t, r.d, r.pid, code, "SMP", "", "automated", specGQL)
		r.state(t, code, "fail", "", "x", "boom")
	}
	res := startAndWait(t, r.s, r.pid)
	if res.Status != "ok" {
		t.Fatalf("%s: %s", res.Status, res.Note)
	}
	var open int
	_ = r.d.QueryRow(context.Background(), `SELECT COUNT(*) FROM qa_defect_state WHERE product_id = ? AND pushed_hash IS NULL`, r.pid).Scan(&open)
	if open != 0 {
		t.Errorf("%d states were not delivered after halving", open)
	}
}

func TestTheDailyCapKeepsItemsPendingAndBacksOff(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":0,"unchanged":0,"rejected":[{"caseId":"TC-SMP-008","reason":"defect-limit"}]}`)
	r.state(t, "TC-SMP-008", "xfail", "DEF-101", "t", "e")
	res := startAndWait(t, r.s, r.pid)
	if res.Status != "ok" {
		t.Fatalf("%s %s", res.Status, res.Note)
	}
	var pushed, rejected *string
	_ = r.d.QueryRow(context.Background(), `SELECT pushed_hash, rejected_hash FROM qa_defect_state WHERE product_id = ?`, r.pid).Scan(&pushed, &rejected)
	if pushed != nil || rejected != nil {
		t.Errorf("a capped item must stay pending (pushed %v, rejected %v)", pushed, rejected)
	}
}

func TestTheExcerptAndTitleStayWithinTheLimits(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":1,"unchanged":0,"rejected":[]}`)
	r.state(t, "TC-SMP-008", "fail", "", strings.Repeat("t", 200), strings.Repeat("é", 5000))
	startAndWait(t, r.s, r.pid)
	it := r.defectReqs()[0].items()[0].(map[string]any)
	if n := len(it["excerpt"].(string)); n > 4096 {
		t.Errorf("excerpt is %d bytes", n)
	}
	if n := len([]rune(it["title"].(string))); n > 200 {
		t.Errorf("title is %d runes", n)
	}
}

func TestAnItemWithoutAUsableSpecIsDroppedLocally(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":1,"unchanged":0,"rejected":[]}`)
	exec(t, r.d, `UPDATE kb_test_cases SET spec_paths = '[]' WHERE product_id = ?`, r.pid)
	r.state(t, "TC-SMP-008", "fail", "", "t", "e")
	startAndWait(t, r.s, r.pid)
	if n := len(r.defectReqs()); n != 0 {
		t.Errorf("an item with no spec was sent")
	}
	var reason string
	_ = r.d.QueryRow(context.Background(), `SELECT rejected_reason FROM qa_defect_state WHERE product_id = ?`, r.pid).Scan(&reason)
	if reason != "no-usable-spec" {
		t.Errorf("reason %q", reason)
	}
}

func TestSweeperPromotesAProductWithPendingDefects(t *testing.T) {
	r := newDefectRig(t)
	r.f.respond = answerDefects(`{"applied":1,"unchanged":0,"rejected":[]}`)
	startAndWait(t, r.s, r.pid) // up to date
	r.state(t, "TC-SMP-008", "fail", "", "t", "e")
	// Everything else is older than the last push, so only the pending defect can owe one.
	exec(t, r.d, `UPDATE kb_test_cases SET updated_at = DATE_SUB(UTC_TIMESTAMP(), INTERVAL 2 HOUR) WHERE product_id = ?`, r.pid)
	exec(t, r.d, `UPDATE kb_case_results SET updated_at = DATE_SUB(UTC_TIMESTAMP(), INTERVAL 2 HOUR) WHERE product_id = ?`, r.pid)
	exec(t, r.d, `UPDATE product_settings SET gaos_last_sync_at = DATE_SUB(UTC_TIMESTAMP(), INTERVAL 1 HOUR) WHERE product_id = ?`, r.pid)
	r.s.sweepOnce(context.Background())
	waitFor(t, "the sweeper pushes the pending defect", func() bool { return len(r.defectReqs()) == 1 })
}

func TestCaseBlockIsTheCasesOwnPart(t *testing.T) {
	text := "head TC-A-001 first body editThing TC-A-002 second body other TC-A-003 third"
	if got := caseBlock(text, "TC-A-002"); got != "TC-A-002 second body other " {
		t.Errorf("block = %q", got)
	}
	if got := caseBlock(text, "TC-A-003"); got != "TC-A-003 third" {
		t.Errorf("the last block runs to the end: %q", got)
	}
	if got := caseBlock(text, "TC-Z-009"); got != text {
		t.Errorf("an unknown code gives the whole text: %q", got)
	}
}
