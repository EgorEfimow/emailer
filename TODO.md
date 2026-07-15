# TODO — Deferred Milestones

Items intentionally out of the active planning. They are tracked here so
contributors can find them without having to read `planning.md`, which only
reflects work scheduled into the current iteration.

If you remove a sub-step from `planning.md` for sanity, migrate it here in
the same commit (see `AGENTS.md` §13).

## Providers

- [x] Ollama (`internal/llm/ollama`)
- [x] OpenRouter (`internal/llm/openrouter`)

## Packaging

- [ ] Dockerfile (`golang:1.25-alpine` build → `distroless/static` runtime)
- [ ] `.dockerignore`

## Digest features (planning.md Phases 8–12)

- [x] "What changed" highlights block (Phase 8)
- [ ] LLM response schema versioning (Phase 9)
- [x] Robust partial LLM failure fallback (Phase 10)
- [x] Digest configuration options (Phase 12)

## Telegram handling (skipped from planning.md Phase 11)

- [ ] Configurable renderer limits: max detailed emails, max summary length, max key points/action items per email, max rendered digest length
- [ ] In Telegram channel, split oversized payloads or fall back to a document when over limits
- [ ] Add truncation indicators and keep MarkdownV2 valid after splitting/truncation
- [ ] Tests covering truncation and oversized-digest delivery behavior
