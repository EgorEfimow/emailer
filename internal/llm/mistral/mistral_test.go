//nolint:errcheck
package mistral

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

// loadFixture reads a JSON fixture file from testdata/mistral/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "mistral", name))
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
	provider, err := Factory(context.Background(), "mistral-test-key", "", "mistral-large-latest")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("Factory() returned nil provider")
	}
	if provider.Name() != "mistral" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "mistral")
	}

	p := provider.(*Provider)
	if p.apiKey != "mistral-test-key" {
		t.Errorf("apiKey = %q, want %q", p.apiKey, "mistral-test-key")
	}
	if p.model != "mistral-large-latest" {
		t.Errorf("model = %q, want %q", p.model, "mistral-large-latest")
	}
	if p.endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", p.endpoint, DefaultEndpoint)
	}
}

func TestFactory_EmptyAPIKey(t *testing.T) {
	_, err := Factory(context.Background(), "", "", "mistral-large-latest")
	if err == nil {
		t.Fatal("Factory() expected error for empty API key")
	}
}

func TestFactory_EmptyModel(t *testing.T) {
	_, err := Factory(context.Background(), "mistral-test-key", "", "")
	if err == nil {
		t.Fatal("Factory() expected error for empty model")
	}
}

func TestFactory_CustomEndpoint(t *testing.T) {
	provider, err := Factory(context.Background(), "mistral-test-key", "https://custom.example.com", "mistral-large-latest")
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

func TestClassify_HappyPath(t *testing.T) { //nolint:gocyclo
	fixture := loadFixture(t, "classify_response.json")

	srv, captured := newTestServer(t, http.StatusOK, fixture)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	req := llm.Request{
		Model:  "mistral-large-latest",
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

	// Verify API key is in the Authorization: Bearer header, not in the URL.
	if *captured != nil {
		auth := (*captured).Header.Get("Authorization")
		if auth == "" {
			t.Error("Authorization header is empty")
		}
		if auth != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-key")
		}
		// Verify key is not in URL.
		if (*captured).URL.RawQuery != "" {
			t.Errorf("API key found in URL query: %s", (*captured).URL.RawQuery)
		}
	}
}

func TestClassify_AuthBearerHeader(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "mistral-secret-key", srv.URL, "mistral-large-latest")
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

	// Verify key is in Authorization: Bearer header, not in URL.
	auth := capturedReq.Header.Get("Authorization")
	if auth != "Bearer mistral-secret-key" {
		t.Errorf("Authorization header = %q, want %q", auth, "Bearer mistral-secret-key")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

func TestClassify_EmptyChoices(t *testing.T) {
	fixture := loadFixture(t, "empty_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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
		t.Fatal("Classify() expected error for empty choices")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

func TestClassify_TokenUsageEmptyResponse(t *testing.T) {
	// Even with empty choices, token usage should be extracted.
	fixture := loadFixture(t, "empty_response.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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
		t.Fatal("Classify() expected error for empty choices")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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
	fixture := loadFixture(t, "classify_response.json")

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the request body to inspect it.
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

	// Verify a system message was sent in the messages array.
	messages, ok := capturedBody["messages"].([]any)
	if !ok {
		t.Fatal("request body missing messages field or not an array")
	}
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(messages))
	}

	sysMsg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatal("messages[0] is not an object")
	}
	if sysMsg["role"] != "system" {
		t.Errorf("messages[0].role = %q, want %q", sysMsg["role"], "system")
	}
	if sysMsg["content"] != "You are a helpful email classifier." {
		t.Errorf("messages[0].content = %q, want %q", sysMsg["content"], "You are a helpful email classifier.")
	}

	// Second message must be the user prompt.
	userMsg, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatal("messages[1] is not an object")
	}
	if userMsg["role"] != "user" {
		t.Errorf("messages[1].role = %q, want %q", userMsg["role"], "user")
	}
}

func TestClassify_WithoutSystemPrompt(t *testing.T) {
	fixture := loadFixture(t, "classify_response.json")

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

	// Verify only a single user message is present (no system message).
	messages, ok := capturedBody["messages"].([]any)
	if !ok {
		t.Fatal("request body missing messages field or not an array")
	}
	if len(messages) != 1 {
		t.Fatalf("expected exactly 1 message (user only), got %d", len(messages))
	}

	userMsg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatal("messages[0] is not an object")
	}
	if userMsg["role"] != "user" {
		t.Errorf("messages[0].role = %q, want %q (no system message expected)", userMsg["role"], "user")
	}
}

// ---------------------------------------------------------------------------
// Name method
// ---------------------------------------------------------------------------

func TestProvider_Name(t *testing.T) {
	provider, err := Factory(context.Background(), "mistral-key", "", "mistral-large-latest")
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if provider.Name() != "mistral" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "mistral")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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
	// Response without a usage block.
	body := []byte(`{
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "{\"schema_version\": 1, \"classifications\": [{\"uid\": 1, \"account\": \"test\", \"label\": \"Useful\", \"confidence\": 0.9, \"reason\": \"ok\", \"summary\": \"Email summary\", \"key_points\": [\"Key point\"], \"action_items\": [], \"priority\": \"medium\"}]}"
				},
				"finish_reason": "stop"
			}
		]
	}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	provider, err := Factory(context.Background(), "test-key", srv.URL, "mistral-large-latest")
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