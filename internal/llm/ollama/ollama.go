// Package ollama implements the llm.Provider interface for Ollama's local LLM API.
//
// Authentication: Ollama typically runs locally without authentication. The API
// key parameter is accepted for interface compatibility but not required.
// The provider uses the /api/chat endpoint with a non-streaming request.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/egorefimow/emailer/internal/llm"
)

const (
	// DefaultEndpoint is the default Ollama API base URL.
	DefaultEndpoint = "http://localhost:11434/api"

	// defaultTimeout is the per-request timeout for Ollama API calls.
	defaultTimeout = 120 * time.Second
)

// ---------------------------------------------------------------------------
// Ollama request types
// ---------------------------------------------------------------------------

// ollamaRequest is the request body for the /api/chat endpoint.
type ollamaRequest struct {
	Model    string        `json:"model"`
	Messages []ollamaMsg   `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  *ollamaOpts   `json:"options,omitempty"`
}

// ollamaMsg represents a single message in the chat conversation.
type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaOpts holds optional generation parameters.
type ollamaOpts struct {
	Temperature float64 `json:"temperature,omitempty"`
}

// ---------------------------------------------------------------------------
// Ollama response types
// ---------------------------------------------------------------------------

// ollamaResponse is the response from the /api/chat endpoint.
type ollamaResponse struct {
	Model              string    `json:"model"`
	Message            ollamaMsg `json:"message"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	EvalCount          int       `json:"eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalDuration       int64     `json:"eval_duration"`
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

// Provider implements the llm.Provider interface for the Ollama API.
type Provider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// compile-time check: *Provider satisfies llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// Factory creates a new Ollama Provider. It is registered with the
// llm.ProviderRegistry and invoked when building the pipeline.
//
// Required parameters:
//   - model: Ollama model identifier (e.g. "llama3.2", "mistral").
//
// Optional parameters:
//   - apiKey: accepted for interface compatibility but not used by Ollama.
//   - endpoint: base URL override; defaults to DefaultEndpoint.
func Factory(_ context.Context, apiKey, endpoint, model string) (llm.Provider, error) {
	if model == "" {
		return nil, fmt.Errorf("ollama: model name is required")
	}
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	return &Provider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}, nil
}

// Name returns the provider name used for registry lookup.
func (p *Provider) Name() string {
	return "ollama"
}

// Classify sends a classification request to the Ollama API and returns
// the parsed response with token usage information.
func (p *Provider) Classify(ctx context.Context, req llm.Request) (llm.Response, error) {
	// Build the classification prompt (labels + emails).
	prompt, err := llm.BuildPrompt(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("ollama.classify.build_prompt: %w", err)
	}

	// Build the Ollama request body.
	oReq := buildRequest(p.model, req.SystemPrompt, prompt)

	body, err := json.Marshal(oReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("ollama.classify.marshal: %w", err)
	}

	// Create the HTTP request.
	url := p.endpoint + "/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, fmt.Errorf("ollama.classify.new_request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send the request.
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("ollama.classify.request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("ollama.classify.read_body: %w", err)
	}

	// Check for non-200 status codes.
	if httpResp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf(
			"ollama.classify: %s: %s", httpResp.Status, string(respBody),
		)
	}

	// Parse the Ollama response.
	var oResp ollamaResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return llm.Response{}, fmt.Errorf("ollama.classify.unmarshal: %w", err)
	}

	if !oResp.Done {
		return llm.Response{}, fmt.Errorf("ollama.classify: response not complete")
	}

	// Extract the text from the response message.
	text := oResp.Message.Content
	if text == "" {
		return llm.Response{TokenUsage: extractTokenUsage(oResp)},
			fmt.Errorf("ollama.classify: empty response content")
	}

	// Parse the classifications from the response text.
	parseResult, err := llm.ParseResponse(text, req.Labels)
	if err != nil {
		// Return raw response for potential repair.
		return llm.Response{
			RawResponse: text,
			TokenUsage:  extractTokenUsage(oResp),
		}, fmt.Errorf("ollama.classify.parse: %w", err)
	}

	return llm.Response{
		Classifications: parseResult.Classifications,
		SchemaVersion:   parseResult.SchemaVersion,
		RawResponse:     text,
		TokenUsage:      extractTokenUsage(oResp),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildRequest constructs the Ollama API request body from the system prompt
// and classification prompt.
func buildRequest(model, systemPrompt, classificationPrompt string) ollamaRequest {
	messages := make([]ollamaMsg, 0, 2)

	if systemPrompt != "" {
		messages = append(messages, ollamaMsg{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	messages = append(messages, ollamaMsg{
		Role:    "user",
		Content: classificationPrompt,
	})

	return ollamaRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Options: &ollamaOpts{
			Temperature: 0,
		},
	}
}

// extractTokenUsage converts Ollama's token counts into the llm.TokenUsage type.
func extractTokenUsage(resp ollamaResponse) llm.TokenUsage {
	if resp.PromptEvalCount == 0 && resp.EvalCount == 0 {
		return llm.TokenUsage{}
	}
	return llm.TokenUsage{
		PromptTokens:     resp.PromptEvalCount,
		CompletionTokens: resp.EvalCount,
		TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
	}
}