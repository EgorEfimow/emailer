# LLM Providers

The Email AI Agent supports multiple LLM providers through a pluggable
provider registry. Each provider implements the `Provider` interface:

```go
type Provider interface {
    Name() string
    Classify(ctx, Request) (Response, error)
}
```

## Supported Providers

| Provider | Identifier | API Key Location | Model Example |
|----------|-----------|-----------------|---------------|
| Gemini | `gemini` | `x-goog-api-key` header | `gemini-2.0-flash` |
| Ollama | `ollama` | not required (local) | `llama3.2` |
| OpenRouter | `openrouter` | `Authorization: Bearer` header | `openai/gpt-4o` |
| Mistral | `mistral` | `Authorization: Bearer` header | `mistral-large-latest` |

> Built-in providers: **Gemini**, **Ollama**, **OpenRouter**, **Mistral**. Each follows the
> same registry pattern described below.

## Gemini

Gemini is the default provider. It uses Google's Gemini API.

### Setup

1. Get an API key from [Google AI Studio](https://aistudio.google.com/).
2. Configure the agent:

```yaml
llm:
  provider: gemini
  api_key: "your-gemini-api-key"
  model: gemini-2.0-flash
```

Or via environment:

```bash
export EMAILER_LLM_PROVIDER=gemini
export EMAILER_LLM_API_KEY=your-gemini-api-key
export EMAILER_LLM_MODEL=gemini-2.0-flash
```

### Supported Models

- `gemini-2.0-flash` (default)
- `gemini-2.0-pro`
- `gemini-1.5-pro`
- `gemini-1.5-flash`

### API Key Security

The API key is sent via the `x-goog-api-key` header — never in the URL.
It is redacted from all logs.

## Ollama

Ollama connects to a local or remote Ollama instance.

### Setup

1. Install [Ollama](https://ollama.com/) and pull a model:
   ```bash
   ollama pull llama3.1
   ```
2. Configure the agent:

```yaml
llm:
  provider: ollama
  api_key: ""  # usually not needed for local instance
  model: llama3.1
  endpoint: "http://localhost:11434"  # optional URL override
```

Or via environment:

```bash
export EMAILER_LLM_PROVIDER=ollama
export EMAILER_LLM_MODEL=llama3.1
export EMAILER_LLM_ENDPOINT=http://localhost:11434
```

### Supported Models

Any model pulled via Ollama, such as:

- `llama3.1`
- `llama3`
- `mistral`
- `mixtral`
- `codellama`
- `phi`

### Notes

- For local Ollama instances, no API key is needed.
- For remote instances, set the `endpoint` to the Ollama server URL.
- The API key, if provided, is sent via the `Authorization` header.

## OpenRouter

OpenRouter provides access to multiple LLM models through a single API.

### Setup

1. Get an API key from [OpenRouter](https://openrouter.ai/).
2. Configure the agent:

```yaml
llm:
  provider: openrouter
  api_key: "sk-or-v1-your-openrouter-key"
  model: openai/gpt-4o
```

Or via environment:

```bash
export EMAILER_LLM_PROVIDER=openrouter
export EMAILER_LLM_API_KEY=sk-or-v1-your-openrouter-key
export EMAILER_LLM_MODEL=openai/gpt-4o
```

### Supported Models

Any model available through OpenRouter, such as:

- `openai/gpt-4o`
- `openai/gpt-4-turbo`
- `anthropic/claude-3.5-sonnet`
- `meta-llama/llama-3.1-405b`

### API Key Security

The API key is sent via the `Authorization: Bearer` header and is redacted
from all logs.

## Mistral

Mistral AI provides an OpenAI-compatible API for their models.

### Setup

1. Get an API key from [Mistral Console](https://console.mistral.ai/).
2. Configure the agent:

```yaml
llm:
  provider: mistral
  api_key: "your-mistral-api-key"
  model: mistral-large-latest
```

Or via environment:

```bash
export EMAILER_LLM_PROVIDER=mistral
export EMAILER_LLM_API_KEY=your-mistral-api-key
export EMAILER_LLM_MODEL=mistral-large-latest
```

### Supported Models

Any model available through the Mistral API, such as:

- `mistral-large-latest`
- `mistral-small-latest`
- `mistral-medium-latest`
- `pixtral-large-latest`

### API Key Security

The API key is sent via the `Authorization: Bearer` header and is redacted
from all logs.

## Adding a New Provider

To add a new LLM provider, follow the steps in [AGENTS.md](../AGENTS.md) §7:

1. Add a constant to the provider enum.
2. Register the provider in the provider registry (in `cmd/emailer/main.go`).
3. Implement the `Provider` interface in `internal/llm/<provider>`.
4. Add HTTP fixtures under `testdata/<provider>/`.
5. Add a contract test that runs against the fixtures.
6. Update `architecture.md` §5.4.
7. Update `.env.example` with provider-specific notes.

## Provider Configuration Reference

| Setting | Description | Default |
|---------|-------------|---------|
| `llm.provider` | Provider identifier | `gemini` |
| `llm.api_key` | API key (sensitive) | — |
| `llm.model` | Model identifier | `gemini-2.0-flash` |
| `llm.endpoint` | Custom API endpoint URL | — |
| `llm.timeout` | Per-request timeout | `120s` |
| `llm.max_retries` | Retry attempts on transient failure | `3` |
| `llm.max_concurrent` | Max concurrent LLM calls | `4` |

## Retry Policy

All providers use the same retry policy:

- **Attempts**: 3 (configurable via `max_retries`)
- **Backoff**: Jittered exponential backoff
  - Base: 1s
  - Factor: 2
  - Jitter: ±25%
- **Retryable statuses**: 429 (rate limit), 5xx (server errors), network errors
- **Non-retryable**: 4xx client errors (except 429), context cancellation, JSON parsing errors