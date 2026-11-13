package testdb

import "testing"

func TestCheckDSNRefusesProductionSchema(t *testing.T) {
	for _, dsn := range []string{
		"u:p@tcp(h:3306)/qa_automation",
		"u:p@tcp(h:3306)/QA_Automation?parseTime=true",
	} {
		if _, err := CheckDSN(dsn); err == nil {
			t.Errorf("%q must be refused", dsn)
		}
	}
	if _, err := CheckDSN("u:p@tcp(h:3306)/scratch"); err != nil {
		t.Errorf("scratch schema must be accepted: %v", err)
	}
	if _, err := CheckDSN("u:p@tcp(h:3306)/"); err != nil {
		t.Errorf("no schema must be accepted: %v", err)
	}
}

func TestNewSkipsWithoutDSN(t *testing.T) {
	t.Setenv(EnvDSN, "")
	t.Run("inner", func(t *testing.T) {
		New(t)
		t.Fatal("must have skipped")
	})
}
