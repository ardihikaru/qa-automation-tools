package api

import (
	"os"

	"github.com/joho/godotenv"
)

func loadIntegrationEnvFiles() {
	candidates := []string{
		".env",
		"../.env",
		"../../.env",
		"../../../.env",
	}

	for _, file := range candidates {
		values, err := godotenv.Read(file)
		if err != nil {
			continue
		}
		for key, value := range values {
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, value)
			}
		}
	}
}

// integrationOptOut returns a skip reason unless QA_INTEGRATION_TESTS=1.
// These tests run migrations and write rows to whatever DB the config points
// at, and the config defaults to the shared production MySQL (10.10.0.4), so
// they never run by accident. Point DB_* / DATABASE_URL at a disposable MySQL
// (see internal/auth/README.md) and set QA_INTEGRATION_TESTS=1.
func integrationOptOut() string {
	if os.Getenv("QA_INTEGRATION_TESTS") != "1" {
		return "Skipping DB integration tests: set QA_INTEGRATION_TESTS=1 and point DB_* at a disposable MySQL"
	}
	return ""
}
