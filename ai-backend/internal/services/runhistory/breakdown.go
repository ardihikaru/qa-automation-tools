package runhistory

import "sort"

// StepSum is the summed time of one step of a job as the SQL returns it: Last is true for the
// group that holds the job's last event (its time up to the job's end is added by the caller).
type StepSum struct {
	Step string
	Last bool
	Ms   int64
}

// StepMs is one step of a build's breakdown.
type StepMs struct {
	Step string `json:"step"`
	Ms   int64  `json:"ms"`
}

// Machine steps of a case_create job (caseauto), in order.
var createOrder = []string{"prepare", "generate", "write", "run", "repair", "approval", "push", "pr", "done"}

// Executor steps of a case_run job, in order.
var runOrder = []string{"validate", "install", "run", "report", "done"}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

// Breakdown folds a job's step sums into its per-step times (RUN_HISTORY.md 11.3). For case_create
// the executor's forwarded steps (validate, install, report, anything outside the machine list)
// and a "done" that is not the job's last event count as "run". Steps are summed, listed in
// canonical order with unknown steps after them alphabetically; zero and negative steps are dropped.
func Breakdown(jobType string, sums []StepSum) []StepMs {
	order := runOrder
	if jobType == "case_create" {
		order = createOrder
	}
	total := map[string]int64{}
	for _, s := range sums {
		step := s.Step
		if jobType == "case_create" && (indexOf(createOrder, step) < 0 || (step == "done" && !s.Last)) {
			step = "run"
		}
		if step == "" {
			step = "run"
		}
		total[step] += s.Ms
	}
	keys := make([]string, 0, len(total))
	for k, v := range total {
		if v > 0 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := indexOf(order, keys[i]), indexOf(order, keys[j])
		switch {
		case a >= 0 && b >= 0:
			return a < b
		case a >= 0:
			return true
		case b >= 0:
			return false
		}
		return keys[i] < keys[j]
	})
	out := make([]StepMs, 0, len(keys))
	for _, k := range keys {
		out = append(out, StepMs{Step: k, Ms: total[k]})
	}
	return out
}

// Clip returns v, or 0 when it is negative (second-precision timestamps can trail millisecond events).
func Clip(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
