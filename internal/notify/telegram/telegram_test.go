//nolint:errcheck
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/notify"
)

// ---------------------------------------------------------------------------
// Helper: loadFixture
// ---------------------------------------------------------------------------

// loadFixture reads a JSON fixture file from testdata/telegram/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "telegram", name))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", name, err)
	}
	return data
}

// ---------------------------------------------------------------------------
// Helper: newTestServer
// ---------------------------------------------------------------------------

// newTestServer creates an httptest.Server that responds with the given
// status code and body, and optionally captures the request for inspection.
func newTestServer(t *testing.T, status int, body []byte) (*httptest.Server, *http.Request) {
	t.Helper()
	var captured http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = *r
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
	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if ch == nil {
		t.Fatal("Factory() returned nil channel")
	}
	if ch.Name() != "telegram" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "telegram")
	}

	c := ch.(*Channel)
	if c.botToken != "123456:ABC-DEF1234" {
		t.Errorf("botToken = %q, want %q", c.botToken, "123456:ABC-DEF1234")
	}
	if c.chatID != -1001234567890 {
		t.Errorf("chatID = %d, want %d", c.chatID, -1001234567890)
	}
	if c.endpoint != defaultEndpoint {
		t.Errorf("endpoint = %q, want %q", c.endpoint, defaultEndpoint)
	}
}

func TestFactory_EmptyBotToken(t *testing.T) {
	cfg := map[string]any{
		"bot_token": "",
		"chat_id":   int64(-1001234567890),
	}

	_, err := Factory(context.Background(), cfg)
	if err == nil {
		t.Fatal("Factory() expected error for empty bot token")
	}
}

func TestFactory_MissingChatID(t *testing.T) {
	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
	}

	_, err := Factory(context.Background(), cfg)
	if err == nil {
		t.Fatal("Factory() expected error for missing chat ID")
	}
}

func TestFactory_CustomEndpoint(t *testing.T) {
	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  "https://custom.example.com",
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	c := ch.(*Channel)
	if c.endpoint != "https://custom.example.com" {
		t.Errorf("endpoint = %q, want %q", c.endpoint, "https://custom.example.com")
	}
}

func TestFactory_UnsupportedConfigType(t *testing.T) {
	_, err := Factory(context.Background(), "string_config")
	if err == nil {
		t.Fatal("Factory() expected error for unsupported config type")
	}
}

// ---------------------------------------------------------------------------
// Send: happy path
// ---------------------------------------------------------------------------

func TestSend_HappyPath(t *testing.T) {
	fixture := loadFixture(t, "send_document_success.json")

	srv, captured := newTestServer(t, http.StatusOK, fixture)
	defer srv.Close()

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	payload := "# Email Digest\n\n**Date:** 2026-07-14\n\nMessage content here."
	opts := notify.SendOptions{
		Filename: "digest-2026-07-14.md",
		Caption:  "📧 3 messages classified — 2 Useful, 1 Ads",
	}

	err = ch.Send(context.Background(), payload, opts)
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}

	// Verify the request was sent to the correct endpoint.
	if captured != nil {
		if !strings.Contains(captured.URL.Path, "sendDocument") {
			t.Errorf("URL path = %q, want .../sendDocument", captured.URL.Path)
		}
		// Verify bot token is in the URL path, not in header or body.
		if !strings.Contains(captured.URL.Path, "123456:ABC-DEF1234") {
			t.Errorf("URL path missing bot token: %q", captured.URL.Path)
		}
		// Verify Content-Type is multipart.
		ct := captured.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/form-data") {
			t.Errorf("Content-Type = %q, want multipart/form-data", ct)
		}
	}
}

func TestSend_NoCaption(t *testing.T) {
	fixture := loadFixture(t, "send_document_success.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Send: error cases
// ---------------------------------------------------------------------------

func TestSend_TooLarge(t *testing.T) {
	ch, err := Factory(context.Background(), map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
	})
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	// Create a payload larger than 45 MB.
	payload := strings.Repeat("x", 46*1024*1024)

	err = ch.Send(context.Background(), payload, notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "45 MB") {
		t.Errorf("error = %q, want '45 MB limit'", err.Error())
	}
}

func TestSend_APIError(t *testing.T) {
	fixture := loadFixture(t, "send_document_server_error.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // Telegram returns 200 even for errors
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for API error")
	}
	if !strings.Contains(err.Error(), "Internal Server Error") {
		t.Errorf("error = %q, want 'Internal Server Error'", err.Error())
	}
}

func TestSend_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":503,"description":"Service Unavailable"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for 503 status")
	}
}

func TestSend_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = ch.Send(ctx, "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Send: retry behaviour
// ---------------------------------------------------------------------------

func TestSend_RetryOn429(t *testing.T) {
	successFixture := loadFixture(t, "send_document_success.json")
	rateLimitFixture := loadFixture(t, "send_document_rate_limited.json")

	var (
		mu        sync.Mutex
		callCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		count := callCount
		mu.Unlock()

		if count == 1 {
			// First call returns 429.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(rateLimitFixture)
			return
		}
		// Subsequent calls succeed.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(successFixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err != nil {
		t.Fatalf("Send() returned error after retry: %v", err)
	}

	mu.Lock()
	got := callCount
	mu.Unlock()
	if got < 2 {
		t.Errorf("expected at least 2 calls (1 retry), got %d", got)
	}
}

func TestSend_RetryOn500(t *testing.T) {
	successFixture := loadFixture(t, "send_document_success.json")

	var (
		mu        sync.Mutex
		callCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		count := callCount
		mu.Unlock()

		if count <= 2 {
			// First two calls fail with 500.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"Internal Server Error"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(successFixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err != nil {
		t.Fatalf("Send() returned error after retry: %v", err)
	}

	mu.Lock()
	got := callCount
	mu.Unlock()
	if got < 3 {
		t.Errorf("expected at least 3 calls (2 retries), got %d", got)
	}
}

func TestSend_RetryExhausted(t *testing.T) {
	rateLimitFixture := loadFixture(t, "send_document_rate_limited.json")

	var (
		mu        sync.Mutex
		callCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rateLimitFixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error after retries exhausted")
	}

	mu.Lock()
	got := callCount
	mu.Unlock()
	if got != maxRetries {
		t.Errorf("expected %d calls (max retries), got %d", maxRetries, got)
	}
}

// ---------------------------------------------------------------------------
// Send: caption truncation
// ---------------------------------------------------------------------------

func TestSend_CaptionTruncated(t *testing.T) {
	fixture := loadFixture(t, "send_document_success.json")

	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the multipart body to inspect caption.
		if err := r.ParseMultipartForm(10 << 20); err == nil {
			capturedBody = r.FormValue("caption")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	// Create a caption longer than 1024 characters.
	longCaption := strings.Repeat("a", 1500)
	opts := notify.SendOptions{Caption: longCaption}

	err = ch.Send(context.Background(), "payload", opts)
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}

	if len(capturedBody) > maxCaptionLength {
		t.Errorf("caption length = %d, want <= %d", len(capturedBody), maxCaptionLength)
	}
}

// ---------------------------------------------------------------------------
// Concurrency safety
// ---------------------------------------------------------------------------

func TestSend_ConcurrencySafety(t *testing.T) {
	fixture := loadFixture(t, "send_document_success.json")

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

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	// Send from multiple goroutines concurrently.
	const n = 5
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			errs <- ch.Send(context.Background(), "payload", notify.SendOptions{})
		}()
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Send error: %v", err)
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
// Name method
// ---------------------------------------------------------------------------

func TestChannel_Name(t *testing.T) {
	ch, err := Factory(context.Background(), map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
	})
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	if ch.Name() != "telegram" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "telegram")
	}
}

// ---------------------------------------------------------------------------
// Helper: isRetryable
// ---------------------------------------------------------------------------

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"network error - no such host", fmt.Errorf("no such host: api.example.com"), true},
		{"network error - connection refused", fmt.Errorf("connection refused"), true},
		{"network error - connection reset", fmt.Errorf("connection reset by peer"), true},
		{"network error - timeout", fmt.Errorf("request timeout"), true},
		{"http 429", fmt.Errorf("code 429"), true},
		{"http 500", fmt.Errorf("code 500"), true},
		{"http 502", fmt.Errorf("code 502"), true},
		{"http 503", fmt.Errorf("code 503"), true},
		{"http 400 (not retryable)", fmt.Errorf("code 400"), false},
		{"http 403 (not retryable)", fmt.Errorf("code 403"), false},
		{"http 404 (not retryable)", fmt.Errorf("code 404"), false},
		{"other error", fmt.Errorf("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err)
			if got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: backoff
// ---------------------------------------------------------------------------

func TestBackoff(t *testing.T) {
	// Backoff should return increasing durations.
	d1 := backoff(1)
	d2 := backoff(2)
	d3 := backoff(3)

	if d1 <= 0 {
		t.Errorf("backoff(1) = %v, want > 0", d1)
	}
	if d2 <= d1 {
		t.Errorf("backoff(2) = %v, want > backoff(1) = %v", d2, d1)
	}
	if d3 <= d2 {
		t.Errorf("backoff(3) = %v, want > backoff(2) = %v", d3, d2)
	}

	// Verify jitter: backoff should be within ±25% of the base.
	// base*0.75 <= backoff(1) <= base*1.25
	min1 := time.Duration(float64(retryBase) * 0.75)
	max1 := time.Duration(float64(retryBase) * 1.25)
	if d1 < min1 || d1 > max1 {
		t.Errorf("backoff(1) = %v, want between %v and %v", d1, min1, max1)
	}
}

// ---------------------------------------------------------------------------
// Notify package: registry
// ---------------------------------------------------------------------------

func TestNotifyRegistry(t *testing.T) {
	registry := notify.NewChannelRegistry()

	registry.Register("telegram", Factory)
	registry.Register("test", func(ctx context.Context, cfg any) (notify.Channel, error) {
		return nil, nil
	})

	registered := registry.Registered()
	if len(registered) != 2 {
		t.Errorf("got %d registered channels, want 2", len(registered))
	}

	factory := registry.Lookup("telegram")
	if factory == nil {
		t.Error("Lookup('telegram') returned nil")
	}

	unknown := registry.Lookup("nonexistent")
	if unknown != nil {
		t.Error("Lookup('nonexistent') should return nil")
	}
}

func TestNotifyRegistry_DuplicatePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register() should panic on duplicate")
		}
	}()

	registry := notify.NewChannelRegistry()
	registry.Register("telegram", Factory)
	registry.Register("telegram", Factory) // Should panic.
}

// ---------------------------------------------------------------------------
// Send: network error
// ---------------------------------------------------------------------------

func TestSend_NetworkError(t *testing.T) {
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

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for network error")
	}
}

// ---------------------------------------------------------------------------
// Send: empty payload
// ---------------------------------------------------------------------------

func TestSend_EmptyPayload(t *testing.T) {
	fixture := loadFixture(t, "send_document_success.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "", notify.SendOptions{})
	if err != nil {
		t.Fatalf("Send() with empty payload returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Send: chat_id as float64 (JSON unmarshaling)
// ---------------------------------------------------------------------------

func TestFactory_ChatIDAsFloat64(t *testing.T) {
	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   float64(-1001234567890),
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	c := ch.(*Channel)
	if c.chatID != -1001234567890 {
		t.Errorf("chatID = %d, want %d", c.chatID, -1001234567890)
	}
}

func TestFactory_MissingBotToken(t *testing.T) {
	cfg := map[string]any{
		"chat_id": int64(-1001234567890),
	}
	_, err := Factory(context.Background(), cfg)
	if err == nil {
		t.Fatal("Factory() expected error for missing bot_token")
	}
}

// ---------------------------------------------------------------------------
// Send: large payload near limit
// ---------------------------------------------------------------------------

func TestSend_LargePayloadNearLimit(t *testing.T) {
	// Create a payload just under 45 MB.
	payload := strings.Repeat("x", 45*1024*1024-1)

	fixture := loadFixture(t, "send_document_success.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), payload, notify.SendOptions{})
	if err != nil {
		t.Fatalf("Send() with large payload returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Send: invalid JSON response
// ---------------------------------------------------------------------------

func TestSend_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for invalid JSON response")
	}
}

// ---------------------------------------------------------------------------
// Send: empty response body
// ---------------------------------------------------------------------------

func TestSend_EmptyResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body.
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   int64(-1001234567890),
		"endpoint":  srv.URL,
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}

	err = ch.Send(context.Background(), "payload", notify.SendOptions{})
	if err == nil {
		t.Fatal("Send() expected error for empty response body")
	}
}

// ---------------------------------------------------------------------------
// Send: chat_id as int64 via JSON unmarshaling
// ---------------------------------------------------------------------------

func TestFactory_ChatIDFloat64NonInt(t *testing.T) {
	// chat_id as float64 from JSON unmarshaling.
	cfg := map[string]any{
		"bot_token": "123456:ABC-DEF1234",
		"chat_id":   float64(-100),
	}

	ch, err := Factory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Factory() returned error: %v", err)
	}
	c := ch.(*Channel)
	if c.chatID != -100 {
		t.Errorf("chatID = %d, want %d", c.chatID, -100)
	}
}

// ---------------------------------------------------------------------------
// JSON unmarshaling of telegramResponse
// ---------------------------------------------------------------------------

func TestTelegramResponse_Unmarshal(t *testing.T) {
	successFixture := loadFixture(t, "send_document_success.json")
	var resp telegramResponse
	if err := json.Unmarshal(successFixture, &resp); err != nil {
		t.Fatalf("failed to unmarshal success response: %v", err)
	}
	if !resp.OK {
		t.Error("expected OK = true")
	}
	if resp.Result == nil {
		t.Error("expected non-nil result")
	}
}

func TestTelegramResponse_UnmarshalError(t *testing.T) {
	errFixture := loadFixture(t, "send_document_rate_limited.json")
	var resp telegramResponse
	if err := json.Unmarshal(errFixture, &resp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if resp.OK {
		t.Error("expected OK = false")
	}
	if resp.ErrorCode != 429 {
		t.Errorf("error_code = %d, want 429", resp.ErrorCode)
	}
	if resp.Description == "" {
		t.Error("expected non-empty description")
	}
}