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
		"../../_docs/markdown-results/by-roles/.env",
		"../../../_docs/markdown-results/by-roles/.env",
		"../../../../_docs/markdown-results/by-roles/.env",
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

	if os.Getenv("DATABASE_URL") == "" && os.Getenv("NEON_DB_CONN") != "" {
		_ = os.Setenv("DATABASE_URL", os.Getenv("NEON_DB_CONN"))
	}
}
