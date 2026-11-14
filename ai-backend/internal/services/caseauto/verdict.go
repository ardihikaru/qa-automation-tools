package caseauto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/caseresults"
)

// Verdict is what a case_run executor result says about one case.
type Verdict struct {
	Pass    bool
	Message string
	Passed  int
	Failed  int
	Skipped int
	Report  *caseresults.Report // nil when the result held no usable report
}

func num(m map[string]interface{}, k string) int {
	if f, ok := m[k].(float64); ok {
		return int(f)
	}
	return 0
}

// VerdictFromResult reads the executor's case_run result {exit_code, passed, failed, skipped,
// report, tail} for the case. A spec that holds no test titled with the code fails with
// "no test titled <code> in <spec>" (CASE_AUTOMATION.md section 5).
func VerdictFromResult(result map[string]interface{}, code, spec string, now time.Time) Verdict {
	v := Verdict{Passed: num(result, "passed"), Failed: num(result, "failed"), Skipped: num(result, "skipped")}
	raw, err := json.Marshal(result["report"])
	if err != nil || result["report"] == nil {
		v.Message = "the executor returned no report"
		if tail, _ := result["tail"].(string); tail != "" {
			v.Message += ": " + Clip(tail, 300)
		}
		return v
	}
	rep, err := caseresults.Parse(bytes.NewReader(raw), now)
	if err != nil {
		v.Message = "the report could not be read: " + Clip(err.Error(), 200)
		return v
	}
	v.Report = rep
	res, ok := rep.Results[code]
	switch {
	case !ok:
		v.Message = fmt.Sprintf("no test titled %s in %s", code, spec)
	case res == caseresults.Pass:
		v.Pass = true
		v.Message = fmt.Sprintf("%s passed", code)
	case res == caseresults.Skipped:
		v.Message = fmt.Sprintf("%s was skipped", code)
	default:
		v.Message = fmt.Sprintf("%s failed", code)
	}
	return v
}
