//nolint:errcheck
package ollama

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

// loadFixture reads a JSON fixture file from testdata/ollama/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ollama", name))
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
	provider, err := Factory(context.Background(), "", "", "llama3.2")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("Factory() returned nil provider")
	}
	if provider.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "ollama")
	}

	p := provider.(*Provider)
	if p.model != "llama3.2" {
		t.Errorf("model = %q, want %q", p.model, "llama3.2")
	}
	if p.endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", p.endpoint, DefaultEndpoint)
	}
	if p.apiKey != "" {
		t.Errorf("apiKey = %q, want empty string", p.apiKey)
	}
}

func TestFactory_EmptyModel(t *testing.T) {
	_, err := Factory(context.Background(), "", "", "")
	if err == nil {
		t.Fatal("Factory() expected error for empty model")
	}
}

func TestFactory_CustomEndpoint(t *testing.T) {
	provider, err := Factory(context.Background(), "", "http://custom:11434/api", "llama3.2")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	p := provider.(*Provider)
	if p.endpoint != "http://custom:11434/api" {
		t.Errorf("endpoint = %q, want %q", p.endpoint, "http://custom:11434/api")
	}
}

// ---------------------------------------------------------------------------
// Classify: happy path
// ---------------------------------------------------------------------------

func TestClassify_HappyPath(t *testing.T) { //nolint:gocyclo
	fixture := loadFixture(t, "classify_response.json")

	srv, _ := newTestServer(t, http.StatusOK, fixture)
	defer srv.Close()

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Model:  "llama3.2",
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
	if c1.Summary != "Project is on track for delivery" {
		t.Errorf("first summary = %q, want %q", c1.Summary, "Project is on track for delivery")
	}
	if len(c1.KeyPoints) != 2 {
		t.Errorf("first key_points count = %d, want 2", len(c1.KeyPoints))
	}
	if c1.Priority != "high" {
		t.Errorf("first priority = %q, want %q", c1.Priority, "high")
	}

	// Check second classification.
	c2 := resp.Classifications[1]
	if c2.Key.AccountLabel != "personal" || c2.Key.UID != 43 {
		t.Errorf("second classification key = %s/%d, want personal/43", c2.Key.AccountLabel, c2.Key.UID)
	}
	if c2.Label != "Ads" {
		t.Errorf("second label = %q, want %q", c2.Label, "Ads")
	}
	if c2.Priority != "low" {
		t.Errorf("second priority = %q, want %q", c2.Priority, "low")
	}

	// Verify raw response is non-empty.
	if resp.RawResponse == "" {
		t.Error("RawResponse is empty")
	}
}

func TestClassify_EmptyResponse(t *testing.T) {
	fixture := loadFixture(t, "empty_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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
		t.Fatal("Classify() expected error for empty response")
	}
}

func TestClassify_ErrorResponse(t *testing.T) {
	fixture := loadFixture(t, "error_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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
		t.Fatal("Classify() expected error for 404 response")
	}
}

func TestClassify_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"Rate limit exceeded"}`))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

func TestClassify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal error"}`))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

func TestClassify_ContextCancelled(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for context cancellation.
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

func TestClassify_NetworkError(t *testing.T) {
	// Use a server that closes the connection immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

func TestClassify_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

func TestClassify_EmptyResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body.
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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
// Token usage extraction
// ---------------------------------------------------------------------------

func TestClassify_TokenUsage(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

	// From fixture: prompt_eval_count=342, eval_count=89
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

func TestClassify_TokenUsageEmptyResponse(t *testing.T) {
	// Even with empty response content, token usage should be extracted.
	fixture := loadFixture(t, "empty_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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
		t.Fatal("Classify() expected error for empty response")
	}

	// Token usage should still be populated.
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

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "classify_response.json"))
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

	// Verify system prompt was sent as a system message.
	messages, ok := capturedBody["messages"].([]any)
	if !ok {
		t.Fatal("request body missing messages array")
	}
	foundSystem := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if msgMap["role"] == "system" && msgMap["content"] == "You are a helpful email classifier." {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Error("system message with system prompt not found in request")
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

	provider, err := Factory(context.Background(), "", srv.URL, "llama3.2")
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

	// Verify no system message was sent.
	messages, ok := capturedBody["messages"].([]any)
	if !ok {
		t.Fatal("request body missing messages array")
	}
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if msgMap["role"] == "system" {
			t.Error("request body should not contain system message when no system prompt is set")
		}
	}
}

// ---------------------------------------------------------------------------
// Name method
// ---------------------------------------------------------------------------

func TestProvider_Name(t *testing.T) {
	provider, err := Factory(context.Background(), "", "", "llama3.2")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if provider.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "ollama")
	}
}