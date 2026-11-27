package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// GAOS_RELEASE_B.md §3: the capabilities route behind the guard, kind on the automation route,
// the run-all kind filter, and the two new status fields.

func TestGAOSReleaseBRoutes(t *testing.T) {
	r, d := gaosAutomationFixture(t)
	bearer := "Bearer " + inboundTok

	if w := inboundCall(r, "GET", gaosCapabilitiesPath, "", ``); w.Code != http.StatusUnauthorized {
		t.Errorf("capabilities without a token: %d, want 401", w.Code)
	}
	if w := inboundCall(r, "GET", gaosCapabilitiesPath, bearer, ``); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"kind"`) {
		t.Errorf("capabilities: %d %s", w.Code, w.Body)
	}
	if w := inboundCall(r, "POST", gaosCapabilitiesPath, bearer, ``); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("capabilities POST: %d, want 405", w.Code)
	}

	// An unknown kind is refused, on any mode.
	for _, mode := range []string{"create", "run"} {
		w := inboundCall(r, "POST", gaosAutomationPath, bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"`+mode+`","kind":"frontend"}`)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "kind must be be or fullstack") {
			t.Errorf("%s with kind frontend: %d %s", mode, w.Code, w.Body)
		}
	}
	// A create whose kind does not fit the case's level is refused.
	must(t, d, `UPDATE kb_test_cases SET level = 'WEB' WHERE code = 'TC-AUTH-001'`)
	w := inboundCall(r, "POST", gaosAutomationPath, bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create","kind":"be"}`)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "kind does not match the case level") {
		t.Errorf("create be of a WEB case: %d %s", w.Code, w.Body)
	}
	// A fitting kind passes the new checks (the shared path answers as before).
	w = inboundCall(r, "POST", gaosAutomationPath, bearer, `{"product":"p-6","caseCode":"TC-AUTH-001","mode":"create","kind":"fullstack","auto_merge":true}`)
	if w.Code == http.StatusUnprocessableEntity {
		t.Errorf("create fullstack of a WEB case was refused: %s", w.Body)
	}

	// run-all of kind be runs only API and GQL cases; with none, the usual 409.
	addCase(t, d, "TC-AUTH-002", "AUTH", "planned", "", "P0", `["tests/web/auth/TC-AUTH-002.spec.ts"]`)
	addCase(t, d, "TC-AUTH-003", "AUTH", "planned", "", "P0", `["tests/api/rest/auth/TC-AUTH-003.spec.ts"]`)
	must(t, d, `UPDATE kb_test_cases SET level = 'WEB' WHERE code = 'TC-AUTH-002'`)
	must(t, d, `UPDATE kb_test_cases SET level = 'API' WHERE code = 'TC-AUTH-003'`)
	codes, err := automatedCaseCodes(context.Background(), d, 1, gaosKindLevels("be")...)
	if err != nil || strings.Join(codes, ",") != "TC-AUTH-003" {
		t.Errorf("be run-all lists %v (%v), want TC-AUTH-003", codes, err)
	}
	all, _ := automatedCaseCodes(context.Background(), d, 1)
	if strings.Join(all, ",") != "TC-AUTH-002,TC-AUTH-003" {
		t.Errorf("an unfiltered run-all lists %v", all)
	}
	must(t, d, `UPDATE kb_test_cases SET level = 'WEB' WHERE code = 'TC-AUTH-003'`)
	if w := inboundCall(r, "POST", gaosAutomationPath, bearer, `{"product":"p-6","caseCode":"*","mode":"run","kind":"be"}`); w.Code != http.StatusConflict {
		t.Errorf("be run-all with no backend case: %d, want 409 (%s)", w.Code, w.Body)
	}
	// An active run-all of another kind is not reused.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES ('job-all-fs','gaos_run_all','running','{"product_id":1,"case_code":"*","kind":"fullstack"}', NOW(3))`)
	if w := inboundCall(r, "POST", gaosAutomationPath, bearer, `{"product":"p-6","caseCode":"*","mode":"run","kind":"be"}`); strings.Contains(w.Body.String(), "job-all-fs") {
		t.Errorf("a be run-all reused the fullstack one: %s", w.Body)
	}

	// The status route: pushed_sha and auto_merge once recorded; a finished job that was asked
	// for an auto-merge and never reached the step says refused:restarted.
	must(t, d, `INSERT INTO job_runs (id, type, status, payload, created_at) VALUES
		('job-am','case_create','running','{"product_id":1,"case_code":"TC-AUTH-001","pr_url":"https://github.com/o/r/pull/7","pushed_sha":"0123456789abcdef0123456789abcdef01234567","auto_merge":"enabled","auto_merge_requested":true}', NOW(3)),
		('job-rs','case_create','completed','{"product_id":1,"case_code":"TC-AUTH-001","pr_url":"https://github.com/o/r/pull/8","auto_merge_requested":true}', NOW(3))`)
	w = inboundCall(r, "GET", "/api/v1/gaos/jobs/job-am", bearer, ``)
	if !strings.Contains(w.Body.String(), `"pushed_sha":"0123456789abcdef0123456789abcdef01234567"`) || !strings.Contains(w.Body.String(), `"auto_merge":"enabled"`) {
		t.Errorf("status of a merged-armed job: %s", w.Body)
	}
	w = inboundCall(r, "GET", "/api/v1/gaos/jobs/job-rs", bearer, ``)
	if !strings.Contains(w.Body.String(), `"auto_merge":"refused:restarted"`) {
		t.Errorf("status of a job a restart completed: %s", w.Body)
	}
}

func TestAutoMergeIsAnAutomationRepoWrite(t *testing.T) {
	if writeAllowed(false, true, RepoOpAutoMerge) == nil {
		t.Error("a non-automation repository with the flag may auto-merge")
	}
	if writeAllowed(true, false, RepoOpAutoMerge) != nil {
		t.Error("an automation repository may not auto-merge")
	}
}
