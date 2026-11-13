package db

import (
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestNormalizeDSN_PinsUTC(t *testing.T) {
	for _, in := range []string{
		"u:p@tcp(10.10.0.4:5306)/qa_automation?parseTime=true&loc=Local&multiStatements=true",
		"u:p@tcp(127.0.0.1:3306)/qa",
		"u:p@tcp(h:1)/d?time_zone=%27%2B07%3A00%27&loc=Asia%2FJakarta",
	} {
		out, err := NormalizeDSN(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		c, err := mysql.ParseDSN(out)
		if err != nil {
			t.Fatalf("re-parse %s: %v", out, err)
		}
		if c.Loc != time.UTC || !c.ParseTime || c.Params["time_zone"] != "'+00:00'" {
			t.Errorf("%s → loc=%v parseTime=%v time_zone=%q", in, c.Loc, c.ParseTime, c.Params["time_zone"])
		}
	}
	if _, err := NormalizeDSN("postgres://u@h/d"); err == nil {
		t.Error("non-MySQL DSN must be rejected")
	}
}

func TestNormalizeDSN_KeepsOtherParams(t *testing.T) {
	out, _ := NormalizeDSN("u:p@tcp(h:1)/d?multiStatements=true&charset=utf8mb4")
	c, _ := mysql.ParseDSN(out)
	if !c.MultiStatements || c.User != "u" || c.Passwd != "p" || c.DBName != "d" {
		t.Errorf("lost settings: %+v", c)
	}
}
