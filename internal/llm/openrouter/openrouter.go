// Package openrouter implements the llm.Provider interface for the
// OpenRouter API.
//
// OpenRouter is an OpenAI-compatible LLM gateway. Authentication is via the
// Authorization: Bearer header — the API key is never sent in the URL or query
// string. The provider uses the /chat/completions endpoint with a
// non-streaming request modeled on the OpenAI Chat Completions schema.
package openrouter

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
	// DefaultEndpoint is the default OpenRouter API base URL.
	DefaultEndpoint = "https://openrouter.ai/api/v1"

	// defaultTimeout is the per-request timeout for OpenRouter API calls.
	defaultTimeout = 120 * time.Second
)

// ---------------------------------------------------------------------------
// OpenRouter request types (OpenAI Chat Completions schema)
// ---------------------------------------------------------------------------

// openrouterRequest is the request body for the /chat/completions endpoint.
type openrouterRequest struct {
	Model       string          `json:"model"`
	Messages    []openrouterMsg `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
}

// openrouterMsg represents a single message in the chat conversation.
type openrouterMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ---------------------------------------------------------------------------
// OpenRouter response types (OpenAI Chat Completions schema)
// ---------------------------------------------------------------------------

// openrouterResponse is the response from the /chat/completions endpoint.
type openrouterResponse struct {
	Choices []openrouterChoice `json:"choices"`
	Usage   *openrouterUsage   `json:"usage,omitempty"`
}

// openrouterChoice is a single completion choice.
type openrouterChoice struct {
	Index        int            `json:"index"`
	Message      openrouterMsg  `json:"message"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

// openrouterUsage reports token consumption for the request.
type openrouterUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

// Provider implements the llm.Provider interface for the OpenRouter API.
type Provider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// compile-time check: *Provider satisfies llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// Factory creates a new OpenRouter Provider. It is registered with the
// llm.ProviderRegistry and invoked when building the pipeline.
//
// Required parameters:
//   - apiKey: OpenRouter API key (e.g. "sk-or-v1-...").
//   - model: OpenRouter model identifier (e.g. "openai/gpt-4o").
//
// Optional parameters:
//   - endpoint: base URL override; defaults to DefaultEndpoint.
func Factory(_ context.Context, apiKey, endpoint, model string) (llm.Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openrouter: api key is required")
	}
	if model == "" {
		return nil, fmt.Errorf("openrouter: model name is required")
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
	return "openrouter"
}

// Classify sends a classification request to the OpenRouter API and returns
// the parsed response with token usage information.
func (p *Provider) Classify(ctx context.Context, req llm.Request) (llm.Response, error) {
	// Build the classification prompt (labels + emails).
	prompt, err := llm.BuildPrompt(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter.classify.build_prompt: %w", err)
	}

	// Build the OpenRouter request body.
	oReq := buildRequest(p.model, req.SystemPrompt, prompt)

	body, err := json.Marshal(oReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter.classify.marshal: %w", err)
	}

	// Create the HTTP request.
	url := p.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter.classify.new_request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send the request.
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter.classify.request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter.classify.read_body: %w", err)
	}

	// Check for non-200 status codes.
	if httpResp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf(
			"openrouter.classify: %s: %s", httpResp.Status, string(respBody),
		)
	}

	// Parse the OpenRouter response.
	var oResp openrouterResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return llm.Response{}, fmt.Errorf("openrouter.classify.unmarshal: %w", err)
	}

	// OpenRouter returns at least one choice on a successful completion.
	if len(oResp.Choices) == 0 {
		return llm.Response{TokenUsage: extractTokenUsage(oResp)},
			fmt.Errorf("openrouter.classify: no choices in response")
	}

	// Extract the text from the first choice's message.
	text := oResp.Choices[0].Message.Content
	if text == "" {
		return llm.Response{TokenUsage: extractTokenUsage(oResp)},
			fmt.Errorf("openrouter.classify: empty response content")
	}

	// Parse the classifications from the response text.
	parseResult, err := llm.ParseResponse(text, req.Labels)
	if err != nil {
		// Return raw response for potential repair.
		return llm.Response{
			RawResponse: text,
			TokenUsage:  extractTokenUsage(oResp),
		}, fmt.Errorf("openrouter.classify.parse: %w", err)
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

// buildRequest constructs the OpenRouter API request body from the system
// prompt and classification prompt.
func buildRequest(model, systemPrompt, classificationPrompt string) openrouterRequest {
	messages := make([]openrouterMsg, 0, 2)

	if systemPrompt != "" {
		messages = append(messages, openrouterMsg{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	messages = append(messages, openrouterMsg{
		Role:    "user",
		Content: classificationPrompt,
	})

	return openrouterRequest{
		Model:       model,
		Messages:    messages,
		Stream:      false,
		Temperature: 0,
	}
}

// extractTokenUsage converts OpenRouter's usage block into the llm.TokenUsage
// type. Returns the zero value when the response carries no usage metadata.
func extractTokenUsage(resp openrouterResponse) llm.TokenUsage {
	if resp.Usage == nil {
		return llm.TokenUsage{}
	}
	return llm.TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
}
