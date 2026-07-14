package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/llm"
	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// Helper: loadFixture
// ---------------------------------------------------------------------------

// loadFixture reads a JSON fixture file from testdata/gemini/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "gemini", name))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", name, err)
	}
	return data
}

// ---------------------------------------------------------------------------
// Helper: newTestServer
// ---------------------------------------------------------------------------

// newTestServer creates an httptest.Server that responds with the given
// status code and body, and records the request for inspection.
func newTestServer(t *testing.T, status int, body []byte) (*httptest.Server, **http.Request) {
	t.Helper()
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.WriteHeader(status)
		if len(body) > 0 {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

// ---------------------------------------------------------------------------
// Factory tests
// ---------------------------------------------------------------------------

func TestFactory_HappyPath(t *testing.T) {
	provider, err := Factory(context.Background(), "test-api-key", "", "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("Factory() returned nil provider")
	}
	if provider.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "gemini")
	}

	p := provider.(*Provider)
	if p.apiKey != "test-api-key" {
		t.Errorf("apiKey = %q, want %q", p.apiKey, "test-api-key")
	}
	if p.model != "gemini-2.0-flash" {
		t.Errorf("model = %q, want %q", p.model, "gemini-2.0-flash")
	}
	if p.endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", p.endpoint, DefaultEndpoint)
	}
}

func TestFactory_EmptyAPIKey(t *testing.T) {
	_, err := Factory(context.Background(), "", "", "gemini-2.0-flash")
	if err == nil {
		t.Fatal("Factory() expected error for empty API key")
	}
}

func TestFactory_EmptyModel(t *testing.T) {
	_, err := Factory(context.Background(), "test-api-key", "", "")
	if err == nil {
		t.Fatal("Factory() expected error for empty model")
	}
}

func TestFactory_CustomEndpoint(t *testing.T) {
	provider, err := Factory(context.Background(), "test-api-key", "https://custom.example.com", "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	p := provider.(*Provider)
	if p.endpoint != "https://custom.example.com" {
		t.Errorf("endpoint = %q, want %q", p.endpoint, "https://custom.example.com")
	}
}

// ---------------------------------------------------------------------------
// Classify: happy path
// ---------------------------------------------------------------------------

func TestClassify_HappyPath(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	srv, captured := newTestServer(t, http.StatusOK, fixture)
	defer srv.Close()

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Model:  "gemini-2.0-flash",
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{
				Key:     mail.MessageKey{AccountLabel: "work", UID: 42},
				Subject: "Project update",
				From:    "alice@example.com",
				Date:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
				Body:    "The project is on track.",
				IsRead:  true,
			},
			{
				Key:     mail.MessageKey{AccountLabel: "personal", UID: 43},
				Subject: "Special offer",
				From:    "marketing@spam.com",
				Date:    time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC),
				Body:    "Buy now!",
				IsRead:  false,
			},
		},
	}

	resp, err := provider.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}

	// Verify classifications.
	if len(resp.Classifications) != 2 {
		t.Fatalf("got %d classifications, want 2", len(resp.Classifications))
	}

	// Check first classification.
	c1 := resp.Classifications[0]
	if c1.Key.AccountLabel != "work" || c1.Key.UID != 42 {
		t.Errorf("first classification key = %s/%d, want work/42", c1.Key.AccountLabel, c1.Key.UID)
	}
	if c1.Label != "Useful" {
		t.Errorf("first label = %q, want %q", c1.Label, "Useful")
	}
	if c1.Confidence != 0.95 {
		t.Errorf("first confidence = %f, want 0.95", c1.Confidence)
	}

	// Check second classification.
	c2 := resp.Classifications[1]
	if c2.Key.AccountLabel != "personal" || c2.Key.UID != 43 {
		t.Errorf("second classification key = %s/%d, want personal/43", c2.Key.AccountLabel, c2.Key.UID)
	}
	if c2.Label != "Ads" {
		t.Errorf("second label = %q, want %q", c2.Label, "Ads")
	}

	// Verify raw response is non-empty.
	if resp.RawResponse == "" {
		t.Error("RawResponse is empty")
	}

	// Verify API key is in the header, not in the URL.
	if *captured != nil {
		apiKey := (*captured).Header.Get("x-goog-api-key")
		if apiKey == "" {
			t.Error("x-goog-api-key header is empty")
		}
		if apiKey != "test-key" {
			t.Errorf("x-goog-api-key = %q, want %q", apiKey, "test-key")
		}
		// Verify key is not in URL.
		if (*captured).URL.RawQuery != "" {
			t.Errorf("API key found in URL query: %s", (*captured).URL.RawQuery)
		}
	}
}

func TestClassify_APIKeyInHeader(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "my-secret-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: "Test email",
			},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}

	// Verify key is in header, not in URL.
	key := capturedReq.Header.Get("x-goog-api-key")
	if key != "my-secret-key" {
		t.Errorf("x-goog-api-key header = %q, want %q", key, "my-secret-key")
	}

	// The URL should be exactly the path, no query params.
	if capturedReq.URL.RawQuery != "" {
		t.Errorf("URL has query params: %s (API key exposed in URL!)", capturedReq.URL.RawQuery)
	}
}

// ---------------------------------------------------------------------------
// Classify: error cases
// ---------------------------------------------------------------------------

func TestClassify_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for 429 status")
	}
}

func TestClassify_EmptyCandidates(t *testing.T) {
	fixture := loadFixture(t, "empty_candidates_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for empty candidates")
	}
}

func TestClassify_SafetyBlocked(t *testing.T) {
	fixture := loadFixture(t, "safety_blocked_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for safety-blocked response")
	}
}

func TestClassify_ContextCancelled(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for context cancellation.
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(ctx, req)
	if err == nil {
		t.Fatal("Classify() expected error for cancelled context")
	}
}

func TestClassify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Internal error"}}`))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for 500 status")
	}
}

// ---------------------------------------------------------------------------
// Token usage extraction
// ---------------------------------------------------------------------------

func TestClassify_TokenUsage(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "work", UID: 42}, Body: "Test"},
			{Key: mail.MessageKey{AccountLabel: "personal", UID: 43}, Body: "Test"},
		},
	}

	resp, err := provider.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}

	if resp.TokenUsage.PromptTokens != 342 {
		t.Errorf("PromptTokens = %d, want 342", resp.TokenUsage.PromptTokens)
	}
	if resp.TokenUsage.CompletionTokens != 89 {
		t.Errorf("CompletionTokens = %d, want 89", resp.TokenUsage.CompletionTokens)
	}
	if resp.TokenUsage.TotalTokens != 431 {
		t.Errorf("TotalTokens = %d, want 431", resp.TokenUsage.TotalTokens)
	}
}

func TestClassify_TokenUsageEmptyCandidates(t *testing.T) {
	// Even with empty candidates, token usage should be extracted.
	fixture := loadFixture(t, "empty_candidates_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	resp, err := provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for empty candidates")
	}

	// Token usage should still be populated even on error.
	if resp.TokenUsage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", resp.TokenUsage.PromptTokens)
	}
	if resp.TokenUsage.TotalTokens != 100 {
		t.Errorf("TotalTokens = %d, want 100", resp.TokenUsage.TotalTokens)
	}
}

// ---------------------------------------------------------------------------
// Concurrency safety
// ---------------------------------------------------------------------------

func TestClassify_ConcurrencySafety(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	var (
		mu        sync.Mutex
		callCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	// Call from multiple goroutines concurrently.
	const n = 5
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := provider.Classify(context.Background(), req)
			errs <- err
		}()
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Classify error: %v", err)
		}
	}

	mu.Lock()
	got := callCount
	mu.Unlock()
	if got != n {
		t.Errorf("expected %d server calls, got %d", n, got)
	}
}

// ---------------------------------------------------------------------------
// System prompt integration
// ---------------------------------------------------------------------------

func TestClassify_WithSystemPrompt(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the request body to inspect it.
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "classify_response.json"))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		SystemPrompt: "You are a helpful email classifier.",
		Labels:       []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}

	// Verify system_instruction was sent.
	sysInstruction, ok := capturedBody["system_instruction"]
	if !ok {
		t.Fatal("request body missing system_instruction field")
	}
	sysMap, ok := sysInstruction.(map[string]any)
	if !ok {
		t.Fatal("system_instruction is not an object")
	}
	parts, ok := sysMap["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatal("system_instruction.parts is empty or not an array")
	}
	part0, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatal("system_instruction.parts[0] is not an object")
	}
	if part0["text"] != "You are a helpful email classifier." {
		t.Errorf("system instruction text = %q, want %q", part0["text"], "You are a helpful email classifier.")
	}
}

func TestClassify_WithoutSystemPrompt(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "classify_response.json"))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}

	// Verify system_instruction is NOT present.
	if _, ok := capturedBody["system_instruction"]; ok {
		t.Error("request body should not contain system_instruction when no system prompt is set")
	}
}

// ---------------------------------------------------------------------------
// Name method
// ---------------------------------------------------------------------------

func TestProvider_Name(t *testing.T) {
	provider, err := Factory(context.Background(), "key", "", "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if provider.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "gemini")
	}
}

// ---------------------------------------------------------------------------
// Error handling: invalid JSON response
// ---------------------------------------------------------------------------

func TestClassify_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for invalid JSON response")
	}
}

func TestClassify_NetworkError(t *testing.T) {
	// Use a server that closes immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for network error")
	}
}

func TestClassify_EmptyResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body.
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	_, err = provider.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("Classify() expected error for empty response body")
	}
}

// ---------------------------------------------------------------------------
// Malformed response: no usage metadata
// ---------------------------------------------------------------------------

func TestClassify_NoUsageMetadata(t *testing.T) {
	// Response without usageMetadata field.
	body := []byte(`{
		"candidates": [
			{
				"content": {
					"parts": [{"text": "{\"classifications\": [{\"uid\": 1, \"account\": \"test\", \"label\": \"Useful\", \"confidence\": 0.9, \"reason\": \"ok\"}]}"}],
					"role": "model"
				},
				"finishReason": "STOP"
			}
		]
	}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []llm.InputMessage{
			{Key: mail.MessageKey{AccountLabel: "test", UID: 1}, Body: "Test"},
		},
	}

	resp, err := provider.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("Classify() returned error: %v", err)
	}

	// Token usage should be zero-valued, not nil.
	if resp.TokenUsage.PromptTokens != 0 || resp.TokenUsage.CompletionTokens != 0 || resp.TokenUsage.TotalTokens != 0 {
		t.Errorf("expected zero token usage, got %+v", resp.TokenUsage)
	}
}
