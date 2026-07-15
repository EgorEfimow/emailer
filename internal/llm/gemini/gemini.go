// Package gemini implements the llm.Provider interface for Google's Gemini API.
//
// Authentication is via the x-goog-api-key header — the API key is never sent
// in the URL. The provider uses the native Gemini generateContent endpoint.
package gemini

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
	// DefaultEndpoint is the default Gemini API base URL.
	DefaultEndpoint = "https://generativelanguage.googleapis.com/v1beta"

	// defaultTimeout is the per-request timeout for Gemini API calls.
	defaultTimeout = 120 * time.Second
)

// ---------------------------------------------------------------------------
// Gemini request types
// ---------------------------------------------------------------------------

// geminiRequest is the request body for the generateContent endpoint.
type geminiRequest struct {
	SystemInstruction *contentPart  `json:"system_instruction,omitempty"`
	Contents          []contentPart `json:"contents"`
}

// contentPart holds a list of text parts for a Gemini request.
type contentPart struct {
	Parts []textPart `json:"parts"`
}

// textPart is a single text segment within a content part.
type textPart struct {
	Text string `json:"text"`
}

// ---------------------------------------------------------------------------
// Gemini response types
// ---------------------------------------------------------------------------

// geminiResponse is the response from the generateContent endpoint.
type geminiResponse struct {
	Candidates     []candidate     `json:"candidates"`
	UsageMetadata  *usageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *promptFeedback `json:"promptFeedback,omitempty"`
}

// candidate represents a single generated response candidate.
type candidate struct {
	Content       contentPart    `json:"content"`
	FinishReason  string         `json:"finishReason"`
	SafetyRatings []safetyRating `json:"safetyRatings,omitempty"`
}

// safetyRating represents a safety assessment for a candidate.
type safetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// promptFeedback contains feedback about the prompt (e.g. safety blocks).
type promptFeedback struct {
	BlockReason string `json:"blockReason"`
}

// usageMetadata contains token usage information.
type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

// Provider implements the llm.Provider interface for the Gemini API.
type Provider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// compile-time check: *Provider satisfies llm.Provider.
var _ llm.Provider = (*Provider)(nil)

// Factory creates a new Gemini Provider. It is registered with the
// llm.ProviderRegistry and invoked when building the pipeline.
//
// Required parameters:
//   - apiKey: Google AI API key (non-empty).
//   - model:  Gemini model identifier (e.g. "gemini-2.0-flash").
//
// Optional:
//   - endpoint: base URL override; defaults to DefaultEndpoint.
func Factory(_ context.Context, apiKey, endpoint, model string) (llm.Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini: api key is required")
	}
	if model == "" {
		return nil, fmt.Errorf("gemini: model name is required")
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
	return "gemini"
}

// Classify sends a classification request to the Gemini API and returns
// the parsed response with token usage information.
//
// The API key is sent via the x-goog-api-key header, never in the URL.
// The system prompt is mapped to Gemini's system_instruction field, and
// the classification prompt + emails are sent as the user content.
func (p *Provider) Classify(ctx context.Context, req llm.Request) (llm.Response, error) {
	// Build the classification prompt (labels + emails).
	prompt, err := llm.BuildPrompt(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini.classify.build_prompt: %w", err)
	}

	// Build the Gemini request body.
	gReq := buildRequest(req.SystemPrompt, prompt)

	body, err := json.Marshal(gReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini.classify.marshal: %w", err)
	}

	// Create the HTTP request with the API key in the header.
	url := p.endpoint + "/models/" + p.model + ":generateContent"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini.classify.new_request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.apiKey)

	// Send the request.
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini.classify.request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("gemini.classify.read_body: %w", err)
	}

	// Check for non-200 status codes.
	if httpResp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf(
			"gemini.classify: %s: %s", httpResp.Status, string(respBody),
		)
	}

	// Parse the Gemini response.
	var gResp geminiResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return llm.Response{}, fmt.Errorf("gemini.classify.unmarshal: %w", err)
	}

	// Check for safety blocks or empty candidates.
	if gResp.PromptFeedback != nil && gResp.PromptFeedback.BlockReason != "" {
		return llm.Response{}, fmt.Errorf(
			"gemini.classify: prompt blocked: %s", gResp.PromptFeedback.BlockReason,
		)
	}

	if len(gResp.Candidates) == 0 {
		return llm.Response{TokenUsage: extractTokenUsage(gResp)},
			fmt.Errorf("gemini.classify: no candidates returned")
	}

	// Extract the text from the first candidate.
	text := extractText(gResp.Candidates[0])
	if text == "" {
		return llm.Response{TokenUsage: extractTokenUsage(gResp)},
			fmt.Errorf("gemini.classify: empty candidate text")
	}

	// Check if the candidate was blocked mid-generation.
	if gResp.Candidates[0].FinishReason == "SAFETY" {
		return llm.Response{
			RawResponse: text,
			TokenUsage:  extractTokenUsage(gResp),
		}, fmt.Errorf("gemini.classify: response blocked by safety filters")
	}

	// Parse the classifications from the response text.
	parseResult, err := llm.ParseResponse(text, req.Labels)
	if err != nil {
		// Return raw response for potential repair.
		return llm.Response{
			RawResponse: text,
			TokenUsage:  extractTokenUsage(gResp),
		}, fmt.Errorf("gemini.classify.parse: %w", err)
	}

	return llm.Response{
		Classifications: parseResult.Classifications,
		SchemaVersion:   parseResult.SchemaVersion,
		RawResponse:     text,
		TokenUsage:      extractTokenUsage(gResp),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildRequest constructs the Gemini API request body from the system prompt
// and the classification prompt.
func buildRequest(systemPrompt, classificationPrompt string) geminiRequest {
	req := geminiRequest{
		Contents: []contentPart{
			{
				Parts: []textPart{
					{Text: classificationPrompt},
				},
			},
		},
	}

	if systemPrompt != "" {
		req.SystemInstruction = &contentPart{
			Parts: []textPart{
				{Text: systemPrompt},
			},
		}
	}

	return req
}

// extractText joins all text parts from a candidate into a single string.
func extractText(c candidate) string {
	var result string
	for _, part := range c.Content.Parts {
		result += part.Text
	}
	return result
}

// extractTokenUsage converts Gemini's usage metadata into the llm.TokenUsage type.
func extractTokenUsage(resp geminiResponse) llm.TokenUsage {
	if resp.UsageMetadata == nil {
		return llm.TokenUsage{}
	}
	return llm.TokenUsage{
		PromptTokens:     resp.UsageMetadata.PromptTokenCount,
		CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      resp.UsageMetadata.TotalTokenCount,
	}
}
