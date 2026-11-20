package runhistory

import (
	"reflect"
	"testing"
)

func TestBreakdown(t *testing.T) {
	const s = int64(1000)
	cases := []struct {
		name string
		typ  string
		in   []StepSum
		want []StepMs
	}{
		{
			name: "case_create fold (the design's example)",
			typ:  "case_create",
			in: []StepSum{
				{"prepare", false, 2 * s}, {"generate", false, 40 * s}, {"write", false, 100}, {"run", false, 58 * s},
				{"validate", false, 1 * s}, {"done", false, 200}, {"repair", false, 35 * s}, {"approval", false, 3 * 3600 * s},
				{"push", false, 4 * s}, {"pr", false, 2 * s}, {"done", true, 0},
			},
			want: []StepMs{{"prepare", 2 * s}, {"generate", 40 * s}, {"write", 100}, {"run", 59*s + 200}, {"repair", 35 * s},
				{"approval", 3 * 3600 * s}, {"push", 4 * s}, {"pr", 2 * s}},
		},
		{
			name: "case_create last done kept, install and report fold into run",
			typ:  "case_create",
			in:   []StepSum{{"install", false, 5}, {"report", false, 6}, {"done", true, 7}, {"weird", false, 8}},
			want: []StepMs{{"run", 19}, {"done", 7}},
		},
		{
			name: "case_run order, unknown steps last alphabetically",
			typ:  "case_run",
			in:   []StepSum{{"done", true, 1}, {"zeta", false, 2}, {"report", false, 3}, {"alpha", false, 4}, {"run", false, 5}, {"validate", false, 6}},
			want: []StepMs{{"validate", 6}, {"run", 5}, {"report", 3}, {"done", 1}, {"alpha", 4}, {"zeta", 2}},
		},
		{name: "no events", typ: "case_create", in: nil, want: []StepMs{}},
		{name: "zero and negative dropped", typ: "case_run", in: []StepSum{{"run", false, 0}, {"done", true, -3}}, want: []StepMs{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Breakdown(c.typ, c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestClip(t *testing.T) {
	if Clip(-1) != 0 || Clip(5) != 5 {
		t.Fatal("clip")
	}
}
