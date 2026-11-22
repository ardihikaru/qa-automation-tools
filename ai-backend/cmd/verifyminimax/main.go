package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// TestResult represents a single test result
type TestResult struct {
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	Endpoint string `json:"endpoint"`
	Success  bool   `json:"success"`
	Status   int    `json:"status"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
	Tokens   int    `json:"tokens,omitempty"`
	Duration int64  `json:"duration_ms"`
	Retry    int    `json:"retry_count"`
}

// TestConfig holds test configuration
type TestConfig struct {
	APIKey     string   `json:"api_key"`
	Models     []string `json:"models"`
	BaseURLs   []string `json:"base_urls"`
	MaxRetries int      `json:"max_retries"`
}

func main() {
	// Read APIKey from environment or first argument
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if len(os.Args) > 1 {
		apiKey = os.Args[1]
	}

	if apiKey == "" {
		fmt.Println("❌ Error: MINIMAX_API_KEY not set")
		fmt.Println("Usage: go run main.go <api_key>")
		fmt.Println("Or set MINIMAX_API_KEY environment variable")
		os.Exit(1)
	}

	// Mask API key for display
	maskedKey := apiKey
	if len(apiKey) > 10 {
		maskedKey = apiKey[:10] + "..."
	}

	// Configuration per MiniMax-API-Endpoint.pdf
	config := TestConfig{
		APIKey: apiKey,
		Models: []string{
			"MiniMax-M2.7",
			"MiniMax-M2.7-highspeed",
			"MiniMax-M2.5",
			"abab6.5s-chat",
		},
		BaseURLs: []string{
			"https://api.minimax.io/v1",
			"https://api.minimax.chat/v1",
		},
		MaxRetries: 4,
	}

	fmt.Println("========================================")
	fmt.Println("🔬 MiniMax API Verification Script")
	fmt.Println("========================================")
	fmt.Printf("API Key: %s\n", maskedKey)
	fmt.Printf("Models to test: %v\n", config.Models)
	fmt.Printf("Base URLs to test: %v\n", config.BaseURLs)
	fmt.Printf("Max retries: %d\n", config.MaxRetries)
	fmt.Println()

	var results []TestResult

	// Test Chat Completion endpoints
	fmt.Println("🧪 Testing Chat Completion Endpoints...")
	fmt.Println("----------------------------------------")

	for _, baseURL := range config.BaseURLs {
		for _, model := range config.Models {
			// Test OpenAI-compatible endpoint
			result := testEndpoint(apiKey, baseURL, "/chat/completions", model, config.MaxRetries)
			result.BaseURL = baseURL
			result.Model = model
			result.Endpoint = "/chat/completions"
			results = append(results, result)

			// Test native endpoint
			result2 := testEndpoint(apiKey, baseURL, "/v1/text/chatcompletion_v2", model, config.MaxRetries)
			result2.BaseURL = baseURL
			result2.Model = model
			result2.Endpoint = "/v1/text/chatcompletion_v2"
			results = append(results, result2)

			fmt.Printf("  [%s] %s %s -> %s (Status: %d)\n",
				successIcon(result.Success),
				baseURL,
				model,
				result.Endpoint,
				result.Status,
			)
		}
	}

	// Find best working configuration
	bestResult := findBestResult(results)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("📊 Test Results Summary")
	fmt.Println("========================================")

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	fmt.Printf("Total tests: %d\n", len(results))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", len(results)-successCount)
	fmt.Println()

	if bestResult != nil {
		fmt.Println("✅ Best Working Configuration:")
		fmt.Printf("   Model: %s\n", bestResult.Model)
		fmt.Printf("   Base URL: %s\n", bestResult.BaseURL)
		fmt.Printf("   Endpoint: %s\n", bestResult.Endpoint)
		fmt.Printf("   Status: %d\n", bestResult.Status)
		fmt.Println()
	} else {
		fmt.Println("❌ No working configuration found!")
		fmt.Println()
	}

	// Write results to JSON
	output := map[string]interface{}{
		"config":  config,
		"results": results,
		"summary": map[string]interface{}{
			"total_tests":   len(results),
			"successful":    successCount,
			"failed":        len(results) - successCount,
			"best_model":    bestResult.Model,
			"best_base_url": bestResult.BaseURL,
			"best_endpoint": bestResult.Endpoint,
		},
	}

	outputJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Printf("❌ Failed to marshal results: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile("result.json", outputJSON, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to write result.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("📁 Results written to result.json")
	fmt.Println("========================================")
}

func testEndpoint(apiKey, baseURL, endpoint, model string, maxRetries int) TestResult {
	result := TestResult{
		Model:    model,
		BaseURL:  baseURL,
		Endpoint: endpoint,
	}

	type ChatRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Temperature float64 `json:"temperature,omitempty"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
	}

	type ChatResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	req := ChatRequest{
		Model: model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: "Hello, please respond with 'OK'"},
		},
		Temperature: 0.7,
		MaxTokens:   1024,
	}

	body, err := json.Marshal(req)
	if err != nil {
		result.Error = fmt.Sprintf("marshal error: %v", err)
		return result
	}

	var lastError error
	var lastStatus int
	var lastResponse string

	for retry := 0; retry <= maxRetries; retry++ {
		result.Retry = retry

		startTime := time.Now()

		httpReq, err := http.NewRequest("POST", baseURL+endpoint, bytes.NewBuffer(body))
		if err != nil {
			lastError = err
			continue
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			lastError = err
			time.Sleep(time.Duration(retry+1) * 1000 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		result.Duration = time.Since(startTime).Milliseconds()
		lastStatus = resp.StatusCode

		respBody, _ := io.ReadAll(resp.Body)
		lastResponse = string(respBody)

		if resp.StatusCode != http.StatusOK {
			// Try to parse error message
			var errResp ChatResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
				lastError = fmt.Errorf("API error: %s", errResp.Error.Message)
			} else {
				lastError = fmt.Errorf("status %d: %s", resp.StatusCode, lastResponse)
			}
			time.Sleep(time.Duration(retry+1) * 1000 * time.Millisecond)
			continue
		}

		var chatResp ChatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			lastError = fmt.Errorf("parse error: %v", err)
			continue
		}

		if len(chatResp.Choices) == 0 {
			lastError = fmt.Errorf("no response choices")
			continue
		}

		result.Success = true
		result.Status = 200
		result.Response = chatResp.Choices[0].Message.Content
		result.Tokens = chatResp.Usage.PromptTokens + chatResp.Usage.CompletionTokens
		return result
	}

	// All retries failed
	result.Success = false
	result.Status = lastStatus
	result.Error = fmt.Sprintf("%v", lastError)
	result.Response = lastResponse

	return result
}

func findBestResult(results []TestResult) *TestResult {
	var best *TestResult

	for i := range results {
		if results[i].Success {
			if best == nil || results[i].Duration < best.Duration {
				best = &results[i]
			}
		}
	}

	return best
}

func successIcon(success bool) string {
	if success {
		return "✅"
	}
	return "❌"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
