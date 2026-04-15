# Senior Software Engineer - Golang Backend

## Overview

Implementation guide for the Golang backend that powers the AI-Powered QA Platform. Handles API endpoints, Minimax AI integration, database operations, and intelligence services.

---

## 1. Project Structure

```
ai-backend/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── handlers.go
│   │   └── router.go
│   ├── services/
│   │   ├── ai/
│   │   │   └── minimax.go
│   │   ├── learning/
│   │   │   └── cache.go
│   │   ├── skills/
│   │   │   └── generator.go
│   │   ├── patch/
│   │   │   └── generator.go
│   │   └── ast/
│   │       └── parser.go
│   ├── db/
│   │   └── postgres.go
│   └── models/
│       └── models.go
├── web/
│   └── (React dashboard)
├── migrations/
│   └── 001_initial.sql
├── go.mod
├── go.sum
└── docker-compose.yml
```

---

## 2. Core Dependencies

```go
module ai-backend

go 1.21

require (
    github.com/lib/pq v1.10.9
    github.com/jackc/pgx v5.5.1
    github.com/jackc/pgx/v5 v5.5.1
    github.com/minimax日 v1.0.0
    github.com/joho/godotenv v1.5.1
    golang.org/x/crypto v0.18.0
)
```

---

## 3. Database Connection

### 3.1 postgres.go

```go
package db

import (
    "context"
    "os"
    "github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Connect() error {
    connStr := os.Getenv("DATABASE_URL")
    // "postgresql://user:pass@ep-xxx.neon.tech/aisystem?sslmode=require"
    
    config, err := pgxpool.ParseConfig(connStr)
    if err != nil {
        return err
    }

    Pool, err = pgxpool.NewWithConfig(context.Background(), config)
    return err
}

func Close() {
    if Pool != nil {
        Pool.Close()
    }
}
```

---

## 4. API Handlers

### 4.1 Ingest Logs

```go
func IngestFlow(w http.ResponseWriter, r *http.Request) {
    var payload struct {
        Role     string `json:"role"`
        Scenario string `json:"scenario"`
        Steps    []struct {
            Action string `json:"action"`
            Page   string `json:"page"`
        } `json:"steps"`
    }

    json.NewDecoder(r.Body).Decode(&payload)

    // Insert flow
    _, err := db.Pool.Exec(context.Background(), `
        INSERT INTO flows (role_id, name)
        VALUES ($1, $2)
    `, payload.Role, payload.Scenario)

    // Log to request_logs for cost tracking
    db.Pool.Exec(context.Background(), `
        INSERT INTO request_logs (type, feature, tokens, cost_estimate, cache_hit)
        VALUES ($1, $2, $3, $4, $5)
    `, "summary", payload.Scenario, 0, 0, false)
}
```

### 4.2 Get Issues with Fix Status

```go
func GetIssues(w http.ResponseWriter, r *http.Request) {
    filter := r.URL.Query().get("filter") // all|fixed|open

    query := `
        SELECT id, page, action, category, is_fixed, notes, created_at
        FROM raw_issues
    `
    
    switch filter {
    case "fixed":
        query += " WHERE is_fixed = true"
    case "open":
        query += " WHERE is_fixed = false"
    }
    
    query += " ORDER BY created_at DESC"

    rows, _ := db.Pool.Query(context.Background(), query)
    defer rows.Close()

    var issues []map]interface{}
    for rows.Next() {
        var i struct {
            ID        int
            Page      string
            Action   string
            Category string
            IsFixed  bool
            Notes   string
            CreatedAt time.Time
        }
        rows.Scan(&i.ID, &i.Page, &i.Action, &i.Category, &i.IsFixed, &i.Notes, &i.CreatedAt)
        issues = append(issues, map[string]interface{}{
            "id": i.ID, "page": i.Page, "action": i.Action,
            "category": i.Category, "is_fixed": i.IsFixed,
            "notes": i.Notes, "created_at": i.CreatedAt,
        })
    }

    json.NewEncoder(w).Encode(issues)
}
```

### 4.3 Update Fix Status

```go
func UpdateIssueStatus(w http.ResponseWriter, r *http.Request) {
    var payload struct {
        ID      int  `json:"id"`
        IsFixed bool `json:"is_fixed"`
    }

    json.NewDecoder(r.Body).Decode(&payload)

    db.Pool.Exec(context.Background(), `
        UPDATE raw_issues
        SET is_fixed = $1
        WHERE id = $2
    `, payload.IsFixed, payload.ID)

    w.Write([]byte(`{"status":"ok"}`))
}
```

### 4.4 Update Notes

```go
func UpdateIssueNotes(w http.ResponseWriter, r *http.Request) {
    var payload struct {
        ID    int    `json:"id"`
        Notes string `json:"notes"`
    }

    json.NewDecoder(r.Body).Decode(&payload)

    db.Pool.Exec(context.Background(), `
        UPDATE raw_issues
        SET notes = $1
        WHERE id = $2
    `, payload.Notes, payload.ID)

    w.Write([]byte(`{"status":"ok"}`))
}
```

---

## 5. AI Services

### 5.1 Minimax Integration

```go
package ai

import (
    "os"
    "bytes"
    "encoding/json"
    "net/http"
)

const APIURL = "https://api.minimax.io/v1/text/chatcompletion_v2"

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type Request struct {
    Model   string    `json:"model"`
    Messages []Message `json:"messages"`
}

func CallMinimax(prompt string) (string, error) {
    apiKey := os.Getenv("MINIMAX_API_KEY")

    reqBody := Request{
        Model: "abab6.5s-chat",
        Messages: []Message{
            {Role: "system", Content: "You are a senior product analyst."},
            {Role: "user", Content: prompt},
        },
    }

    body, _ := json.Marshal(reqBody)

    req, _ := http.NewRequest("POST", APIURL, bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result map]interface{}
    json.NewDecoder(resp.Body).Decode(&result)

    return result["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string), nil
}
```

### 5.2 Cached Summary

```go
package ai

import (
    "crypto/sha256"
    "encoding/hex"
    "context"
    "ai-backend/internal/db"
)

func hashInput(input string) string {
    h := sha256.Sum256([]byte(input))
    return hex.EncodeToString(h[:])
}

func GetSummary(input string) (string, error) {
    hash := hashInput(input)

    var cached string
    err := db.Pool.QueryRow(context.Background(), `
        SELECT output FROM ai_cache WHERE input_hash = $1
    `, hash).Scan(&cached)

    if err == nil {
        // Cache hit - log it
        db.Pool.Exec(context.Background(), `
            INSERT INTO request_logs (type, tokens, cost_estimate, cache_hit)
            VALUES ('summary', 0, 0, true)
        `)
        return cached, nil
    }

    // Cache miss - call Minimax
    result := CallMinimax(input)

    db.Pool.Exec(context.Background(), `
        INSERT INTO ai_cache (cache_key, input_hash, output)
        VALUES ('summary', $1, $2)
    `, hash, result)

    db.Pool.Exec(context.Background(), `
        INSERT INTO request_logs (type, tokens, cost_estimate, cache_hit)
        VALUES ('summary', 500, 0.001, false)
    `)

    return result, nil
}
```

---

## 6. Learning Service

### 6.1 Query Learning Cache

```go
func SearchLearning(embedding []float32, intent string) []map]interface{} {
    rows, _ := db.Pool.Query(context.Background(), `
        SELECT *,
            (1 - (embedding <-> $1)) AS similarity
        FROM learning_cache
        WHERE intent = $2
        ORDER BY embedding <-> $1
        LIMIT 5
    `, embedding, intent)

    defer rows.Close()

    var results []map]interface{}
    for rows.Next() {
        var id int
        var intent, normalizedInput string
        var successScore float64
        var usageCount int
        var similarity float32

        rows.Scan(&id, &intent, &normalizedInput, nil, nil, &successScore, &usageCount, nil, nil, &similarity)

        results = append(results, map[string]interface{}{
            "id": id, "intent": intent,
            "success_score": successScore,
            "usage_count": usageCount,
            "similarity": similarity,
        })
    }

    return results
}
```

### 6.2 Update Success Score

```go
func UpdateSuccess(action string, success bool) {
    if success {
        db.Pool.Exec(context.Background(), `
            UPDATE learning_cache
            SET 
                success_score = (success_score + 1) / 2,
                usage_count = usage_count + 1,
                last_used = NOW()
            WHERE normalized_input = $1
        `, action)
    } else {
        db.Pool.Exec(context.Background(), `
            UPDATE learning_cache
            SET success_score = success_score * 0.7
            WHERE normalized_input = $1
        `, action)
    }
}
```

---

## 7. Skills Generator Service

### 7.1 Generate Skills from Issues

```go
func GenerateSkills() error {
    // Get repeated issues
    rows, _ := db.Pool.Query(context.Background(), `
        SELECT error_text, COUNT(*)
        FROM raw_issues
        GROUP BY error_text
        HAVING COUNT(*) > 3
    `)
    defer rows.Close()

    for rows.Next() {
        var errorText string
        var count int
        rows.Scan(&errorText, &count)

        // Call Minimax to generate skill
        prompt := `You are a QA expert.
Given these repeated UI issues:
- ` + errorText + `
Generate:
1. skill name
2. description
3. detection logic (JSON)
4. severity level`

        result := ai.CallMinimax(prompt)

        // Parse and store
        // ... (store in skills table)
    }

    return nil
}
```

---

## 8. Patch Generator Service

### 8.1 Generate Diff

```go
func GeneratePatch(issue Issue, filePath string) (string, error) {
    // Read file
    content, err := os.ReadFile(filePath)
    if err != nil {
        return "", err
    }

    prompt := `File: ` + filePath + `
Code:
` + string(content) + `
Issue:
` + issue.ErrorText + `
Task: Generate minimal patch (diff format) to fix this.`

    return ai.CallMinimax(prompt)
}
```

### 8.2 File Detection

```go
func GuessFile(issue Issue) string {
    page := issue.Page

    if strings.Contains(page, "billing") {
        return "frontend/src/pages/Billing.tsx"
    }
    if strings.Contains(page, "user") {
        return "frontend/src/pages/Users.tsx"
    }
    if strings.Contains(issue.ErrorText, "api") {
        return "backend/handlers/api.go"
    }

    return ""
}
```

---

## 9. AST Parser (Go)

### 9.1 Extract Functions

```go
package astutil

import (
    "go/parser"
    "go/token"
    "go/ast"
)

func ExtractFunctions(filePath string) ([]string, error) {
    fset := token.NewFileSet()
    node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
    if err != nil {
        return nil, err
    }

    var funcs []string
    ast.Inspect(node, func(n ast.Node) bool {
        if fn, ok := n.(*ast.FuncDecl); ok {
            funcs = append(funcs, fn.Name.Name)
        }
        return true
    })

    return funcs, nil
}
```

---

## 10. Router Setup

```go
func SetupRouter() *http.ServeMux {
    mux := http.NewServeMux()

    // Flow endpoints
    mux.HandleFunc("/api/flows", IngestFlow)
    mux.HandleFunc("/api/flows", GetFlows)

    // Issue endpoints
    mux.HandleFunc("/api/issues", GetIssues)
    mux.HandleFunc("/api/issues/status", UpdateIssueStatus)
    mux.HandleFunc("/api/issues/notes", UpdateIssueNotes)

    // AI endpoints
    mux.HandleFunc("/api/search", Search)
    mux.HandleFunc("/api/ai/summary", GetSummary)

    // Cost endpoints
    mux.HandleFunc("/api/cost/overview", GetCostOverview)
    mux.HandleFunc("/api/cost/trend", GetCostTrend)

    // Static files
    mux.Handle("/videos/", http.Dir("./output/videos"))

    return mux
}
```

---

## 11. Main Entry Point

```go
package main

import (
    "log"
    "net/http"
    "os"
    "ai-backend/internal/db"
    "ai-backend/internal/api"
)

func main() {
    // Connect to database
    if err := db.Connect(); err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Setup router
    router := api.SetupRouter()

    // Start server
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    log.Printf("Server running on :%s", port)
    http.ListenAndServe(":"+port, router)
}
```

---

## 12. Environment Variables

```bash
# .env file
DATABASE_URL=postgresql://user:pass@ep-xxx.neon.tech/aisystem?sslmode=require
MINIMAX_API_KEY=your_api_key_here
PORT=8080
```

---

## 13. Docker

### 13.1 Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY ../.. .
RUN go build -o server ./cmd/server

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
```

### 13.2 docker-compose.yml

```yaml
version: "3"

services:
  backend:
    build: ./backend
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - MINIMAX_API_KEY=${MINIMAX_API_KEY}
    volumes:
      - ./output:/app/output

  dashboard:
    build: ./dashboard
    ports:
      - "3000:3000"
```

---

## 14. Testing

### 14.1 Unit Test Example

```go
package ai_test

import (
    "testing"
    "ai-backend/internal/ai"
)

func TestHashInput(t *testing.T) {
    input := "test input"
    hash := ai.HashInput(input)
    
    if len(hash) != 64 {
        t.Errorf("expected 64 char hash, got %d", len(hash))
    }
}

func TestCachedSummary(t *testing.T) {
    // Mock database
    // Test cache hit/miss logic
}
```

---

## 15. Deployment

### Build
```bash
cd backend
go build -o server ./cmd/server
```

### Run
```bash
./server
```

### With Docker
```bash
docker-compose up --build
```

---

## Key Implementation Notes

1. **Always use parameterized queries** — prevent SQL injection
2. **Log all AI requests** — for cost tracking
3. **Implement caching first** — before calling Minimax
4. **Validate input** — sanitize all user inputs
5. **Handle errors gracefully** — return proper HTTP codes

---

*Document Version: 1.0*
*Source: Playwright-for-System-Documentation.pdf*