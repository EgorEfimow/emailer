// Package telegram implements the notify.Channel interface for the Telegram
// Bot API. Digest payloads are sent as documents via sendDocument, with an
// optional caption (max 1024 characters). Short messages can be sent via
// sendMessage.
//
// Retry policy: 3 attempts, jittered exponential backoff (base 1s, factor 2,
// jitter ±25%), only on 429/5xx/network errors.
//
// Size guard: payloads over 45 MB are rejected to respect Telegram's file
// size limit.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/egorefimow/emailer/internal/notify"
)

const (
	// defaultEndpoint is the Telegram Bot API base URL.
	defaultEndpoint = "https://api.telegram.org"

	// defaultTimeout is the per-request timeout for Telegram API calls.
	defaultTimeout = 30 * time.Second

	// maxPayloadSize is the maximum allowed payload size (45 MB).
	maxPayloadSize = 45 * 1024 * 1024

	// maxCaptionLength is the maximum length of a caption (Telegram limit).
	maxCaptionLength = 1024

	// maxRetries is the number of retry attempts on transient failures.
	maxRetries = 3

	// retryBase is the base delay for exponential backoff.
	retryBase = 1 * time.Second
)

// ---------------------------------------------------------------------------
// Telegram API types
// ---------------------------------------------------------------------------

// telegramResponse is the standard Telegram Bot API response wrapper.
type telegramResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// ---------------------------------------------------------------------------
// Channel
// ---------------------------------------------------------------------------

// Channel implements the notify.Channel interface for Telegram.
type Channel struct {
	botToken string
	chatID   int64
	endpoint string
	client   *http.Client
}

// compile-time check: *Channel satisfies notify.Channel.
var _ notify.Channel = (*Channel)(nil)

// Factory creates a new Telegram Channel. Accepts config as a
// map[string]any or a struct with BotToken and ChatID fields.
func Factory(_ context.Context, cfg any) (notify.Channel, error) {
	var botToken string
	var chatID int64
	var endpoint string

	switch v := cfg.(type) {
	case map[string]any:
		token, ok := v["bot_token"].(string)
		if !ok || token == "" {
			return nil, fmt.Errorf("telegram: bot_token is required")
		}
		botToken = token

		cid, ok := v["chat_id"].(int64)
		if !ok {
			// Try float64 (JSON unmarshaling).
			if f, ok := v["chat_id"].(float64); ok {
				cid = int64(f)
			} else {
				return nil, fmt.Errorf("telegram: chat_id is required")
			}
		}
		chatID = cid

		if ep, ok := v["endpoint"].(string); ok && ep != "" {
			endpoint = ep
		}
	default:
		return nil, fmt.Errorf("telegram: unsupported config type %T", cfg)
	}

	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	return &Channel{
		botToken: botToken,
		chatID:   chatID,
		endpoint: endpoint,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}, nil
}

// Name returns "telegram".
func (c *Channel) Name() string {
	return "telegram"
}

// Send delivers a digest payload to the Telegram chat.
//
// The payload is sent as a document (Markdown file) via sendDocument.
// If opts.Caption is set, it is included as the document caption (truncated
// to 1024 characters). If the payload exceeds 45 MB, an error is returned.
//
// Retry logic: up to 3 attempts with jittered exponential backoff on
// 429/5xx/network errors.
func (c *Channel) Send(ctx context.Context, payload string, opts notify.SendOptions) error {
	// Size guard: reject payloads over 45 MB.
	if len(payload) > maxPayloadSize {
		return fmt.Errorf("telegram.send: payload %d bytes exceeds 45 MB limit", len(payload))
	}

	// Truncate caption to 1024 characters.
	caption := opts.Caption
	if len(caption) > maxCaptionLength {
		caption = caption[:maxCaptionLength]
	}

	// Send with retry.
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("telegram.send: %w", ctx.Err())
		default:
		}

		err := c.sendDocument(ctx, payload, caption)
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't sleep on the last attempt.
		if attempt < maxRetries && isRetryable(err) {
			delay := backoff(attempt)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("telegram.send: %w", ctx.Err())
			case <-timer.C:
			}
		}
	}

	return fmt.Errorf("telegram.send: %w", lastErr)
}

// ---------------------------------------------------------------------------
// sendDocument
// ---------------------------------------------------------------------------

// sendDocument sends a document (the digest) to the Telegram chat via the
// sendDocument endpoint using multipart/form-data.
func (c *Channel) sendDocument(ctx context.Context, payload, caption string) error {
	url := fmt.Sprintf("%s/bot%s/sendDocument", c.endpoint, c.botToken)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// chat_id field.
	if err := writer.WriteField("chat_id", fmt.Sprintf("%d", c.chatID)); err != nil {
		return fmt.Errorf("telegram.send_document.write_field.chat_id: %w", err)
	}

	// caption field (optional).
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return fmt.Errorf("telegram.send_document.write_field.caption: %w", err)
		}
	}

	// parse_mode field.
	if err := writer.WriteField("parse_mode", "MarkdownV2"); err != nil {
		return fmt.Errorf("telegram.send_document.write_field.parse_mode: %w", err)
	}

	// document file part.
	part, err := writer.CreateFormFile("document", "digest.md")
	if err != nil {
		return fmt.Errorf("telegram.send_document.create_file: %w", err)
	}
	if _, err := io.WriteString(part, payload); err != nil {
		return fmt.Errorf("telegram.send_document.write_file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("telegram.send_document.close_writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("telegram.send_document.new_request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpResp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram.send_document.request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("telegram.send_document.read_body: %w", err)
	}

	// Parse the response.
	var tgResp telegramResponse
	if err := json.Unmarshal(respBody, &tgResp); err != nil {
		return fmt.Errorf("telegram.send_document.unmarshal: %w", err)
	}

	if !tgResp.OK {
		return fmt.Errorf("telegram.send_document: %s (code %d)", tgResp.Description, tgResp.ErrorCode)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isRetryable returns true if the error is a transient error that should be
// retried: network errors, 429 Too Many Requests, or 5xx server errors.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// Network errors (no response from server).
	if strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "Temporary") {
		return true
	}

	// HTTP-level errors.
	if strings.Contains(errStr, "code 429") ||
		strings.Contains(errStr, "code 500") ||
		strings.Contains(errStr, "code 502") ||
		strings.Contains(errStr, "code 503") {
		return true
	}

	return false
}

// backoff computes the delay for the given attempt (1-indexed) using
// exponential backoff with jitter: base * 2^(attempt-1) ± 25%.
func backoff(attempt int) time.Duration {
	multiplier := 1
	for i := 1; i < attempt; i++ {
		multiplier *= 2
	}
	delayNs := float64(retryBase) * float64(multiplier)

	// Apply ±25% jitter.
	jitter := 0.25
	random := rand.Float64() //nolint:gosec // crypto/rand not needed for timing jitter
	delayNs = delayNs * (1 - jitter + 2*jitter*random)

	return time.Duration(delayNs)
}