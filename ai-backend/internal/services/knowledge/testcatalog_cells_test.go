package knowledge

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func canonText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func TestCellsEscaping(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"| a | b |", []string{"a", "b"}},
		{`| a\|b | c |`, []string{"a|b", "c"}},
		{`| a\\|b |`, []string{`a\\`, "b"}}, // even run: delimiter, run kept
		{`| a\\\|b |`, []string{`a\|b`}},    // 3: one backslash then literal pipe
		{`| a\\\\\|b |`, []string{`a\\|b`}}, // 5
		{`| C:\dir\x | \d |`, []string{`C:\dir\x`, `\d`}},
		{"| a<br>b | c<BR/>d | e<br />f |", []string{"a\nb", "c\nd", "e\nf"}},
		{"| a<br  >b |", []string{"a<br  >b"}},
		{"no pipes", nil},
	}
	for _, c := range cases {
		if got := cells(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("cells(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestEncodeCellRefusals(t *testing.T) {
	for _, in := range []string{"a\x00b", "a\tb", "a\rb", "a<br>b", "a<BR/>b", "a<br />b", "a\x7fb"} {
		if _, err := EncodeCell(in); !errors.Is(err, ErrCellText) {
			t.Errorf("EncodeCell(%q) err = %v, want ErrCellText", in, err)
		}
		if _, err := EncodeStoryText(in); !errors.Is(err, ErrCellText) {
			t.Errorf("EncodeStoryText(%q) err = %v, want ErrCellText", in, err)
		}
	}
	if got, err := EncodeCell("a\r\nb "); err != nil || got != "a<br>b" {
		t.Errorf("CRLF: %q %v", got, err)
	}
}

func TestCellRoundTrip(t *testing.T) {
	alphabet := []string{"a", "b", " ", "|", `\`, `\`, "|", "\n", "\r\n", "`", "<", ">", "br", "é", "  "}
	rng := rand.New(rand.NewSource(7))
	fixed := []string{"", `\`, `\\`, `\|`, `\\|`, "|", "||", `a\`, "a|", `|\`, "x\n\ny", " lead", "trail  ", "a `|` b", "C:\\x\\|y"}
	var inputs []string
	inputs = append(inputs, fixed...)
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for n := rng.Intn(12); n > 0; n-- {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		inputs = append(inputs, b.String())
	}
	for _, x := range inputs {
		enc, err := EncodeCell(x)
		if err != nil {
			continue // literal <br> assembled from pieces is refused by design
		}
		if strings.ContainsAny(enc, "\n\r") {
			t.Fatalf("encoded cell has a line break: %q", enc)
		}
		row := "| ☐ | " + enc + " | tail |"
		got := cells(row)
		if len(got) != 3 || got[1] != canonText(x) || got[2] != "tail" {
			t.Fatalf("round trip of %q: encoded %q, parsed %q", x, enc, got)
		}
		// first and last cell position too (trailing backslash next to the final delimiter)
		if g := cells("| " + enc + " |"); len(g) != 1 || g[0] != canonText(x) {
			t.Fatalf("single cell round trip of %q: encoded %q, parsed %q", x, enc, g)
		}
	}
}

func TestStoryTextRoundTrip(t *testing.T) {
	enc, err := EncodeStoryText("as a user | I want\nmore \\")
	if err != nil {
		t.Fatal(err)
	}
	m := storyLineRe.FindStringSubmatch("- **US-A-01** (unmapped: x) — " + enc)
	if m == nil {
		t.Fatalf("story line did not match: %q", enc)
	}
	if got := decodeBR(strings.TrimSpace(m[3])); got != "as a user | I want\nmore \\" {
		t.Errorf("got %q", got)
	}
}

func TestParserVersion(t *testing.T) {
	if v := (testCatalog{}).Version(); v != 4 {
		t.Errorf("Version = %d, want 4", v)
	}
}
