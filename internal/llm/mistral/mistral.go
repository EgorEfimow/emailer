// Package mistral implements the llm.Provider interface for the
// Mistral AI API.
//
// Mistral's API is OpenAI-compatible. Authentication is via the
// Authorization: Bearer header — the API key is never sent in the URL or
// query string. The provider uses the /v1/chat/completions endpoint with a
// non-streaming request modeled on the OpenAI Chat Completions schema.
package mistral

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
	// DefaultEndpoint is the default Mistral AI API base URL.
	DefaultEndpoint = "https://api.mistral.ai/v1"

	// defaultTimeout is the per-request timeout for Mistral API calls.
	defaultTimeout = 120 * time.Second
)

// ---------------------------------------------------------------------------
// Mistral request types (OpenAI Chat Completions schema)
// ---------------------------------------------------------------------------

// mistralRequest is the request body for the /v1/chat/completions endpoint.
type mistralRequest struct {
	Model       string        `json:"model"`
	Messages    []mistralMsg  `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
}

// mistralMsg represents a single message in the chat conversation.
type mistralMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ---------------------------------------------------------------------------
// Mistral response types (OpenAI Chat Completions schema)
// ---------------------------------------------------------------------------

// mistralResponse is the response from the /v1/chat/completions endpoint.
type mistralResponse struct {
	Choices []mistralChoice  `json:"choices"`
	Usage   *mistralUsage    `json:"usage,omitempty"`
}

// mistralChoice is a single completion choice.
type mistralChoice struct {
	Index        int          `json:"index"`
	Message      mistralMsg   `json:"message"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

// mistralUsage reports token consumption for the request.
type mistralUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

// Provider implements the llm.Provider interface for the Mistral AI API.
type Provider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// compile-time check: *Provider satisfies llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// Factory creates a new Mistral Provider. It is registered with the
// llm.ProviderRegistry and invoked when building the pipeline.
//
// Required parameters:
//   - apiKey: Mistral API key.
//   - model: Mistral model identifier (e.g. "mistral-large-latest").
//
// Optional parameters:
//   - endpoint: base URL override; defaults to DefaultEndpoint.
func Factory(_ context.Context, apiKey, endpoint, model string) (llm.Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("mistral: api key is required")
	}
	if model == "" {
		return nil, fmt.Errorf("mistral: model name is required")
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
	return "mistral"
}

// Classify sends a classification request to the Mistral API and returns
// the parsed response with token usage information.
func (p *Provider) Classify(ctx context.Context, req llm.Request) (llm.Response, error) {
	// Build the classification prompt (labels + emails).
	prompt, err := llm.BuildPrompt(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("mistral.classify.build_prompt: %w", err)
	}

	// Build the Mistral request body.
	mReq := buildRequest(p.model, req.SystemPrompt, prompt)

	body, err := json.Marshal(mReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("mistral.classify.marshal: %w", err)
	}

	// Create the HTTP request.
	url := p.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, fmt.Errorf("mistral.classify.new_request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send the request.
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("mistral.classify.request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("mistral.classify.read_body: %w", err)
	}

	// Check for non-200 status codes.
	if httpResp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf(
			"mistral.classify: %s: %s", httpResp.Status, string(respBody),
		)
	}

	// Parse the Mistral response.
	var mResp mistralResponse
	if err := json.Unmarshal(respBody, &mResp); err != nil {
		return llm.Response{}, fmt.Errorf("mistral.classify.unmarshal: %w", err)
	}

	// Mistral returns at least one choice on a successful completion.
	if len(mResp.Choices) == 0 {
		return llm.Response{TokenUsage: extractTokenUsage(mResp)},
			fmt.Errorf("mistral.classify: no choices in response")
	}

	// Extract the text from the first choice's message.
	text := mResp.Choices[0].Message.Content
	if text == "" {
		return llm.Response{TokenUsage: extractTokenUsage(mResp)},
			fmt.Errorf("mistral.classify: empty response content")
	}

	// Parse the classifications from the response text.
	parseResult, err := llm.ParseResponse(text, req.Labels)
	if err != nil {
		// Return raw response for potential repair.
		return llm.Response{
			RawResponse: text,
			TokenUsage:  extractTokenUsage(mResp),
		}, fmt.Errorf("mistral.classify.parse: %w", err)
	}

	return llm.Response{
		Classifications: parseResult.Classifications,
		SchemaVersion:   parseResult.SchemaVersion,
		RawResponse:     text,
		TokenUsage:      extractTokenUsage(mResp),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildRequest constructs the Mistral API request body from the system
// prompt and classification prompt.
func buildRequest(model, systemPrompt, classificationPrompt string) mistralRequest {
	messages := make([]mistralMsg, 0, 2)

	if systemPrompt != "" {
		messages = append(messages, mistralMsg{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	messages = append(messages, mistralMsg{
		Role:    "user",
		Content: classificationPrompt,
	})

	return mistralRequest{
		Model:       model,
		Messages:    messages,
		Stream:      false,
		Temperature: 0,
	}
}

// extractTokenUsage converts Mistral's usage block into the llm.TokenUsage
// type. Returns the zero value when the response carries no usage metadata.
func extractTokenUsage(resp mistralResponse) llm.TokenUsage {
	if resp.Usage == nil {
		return llm.TokenUsage{}
	}
	return llm.TokenUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
}