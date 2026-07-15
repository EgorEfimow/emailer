# TODO — Deferred Milestones

Items intentionally out of the active planning. They are tracked here so
contributors can find them without having to read `planning.md`, which only
reflects work scheduled into the current iteration.

If you remove a sub-step from `planning.md` for sanity, migrate it here in
the same commit (see `AGENTS.md` §13).

## Providers

- [ ] Ollama (`internal/llm/ollama`)
- [ ] OpenRouter (`internal/llm/openrouter`)

## Packaging

- [ ] Dockerfile (`golang:1.25-alpine` build → `distroless/static` runtime)
- [ ] `.dockerignore`

## Digest features (planning.md Phases 8–12)

- [x] "What changed" highlights block (Phase 8)
- [ ] LLM response schema versioning (Phase 9)
- [x] Robust partial LLM failure fallback (Phase 10)
- [ ] Telegram-safe digest length controls (Phase 11)
- [ ] Digest configuration options (Phase 12)

## Release process (planning.md Phase 15)

- [ ] Tag `v0.1.0-rc.1` and cut release candidate
- [ ] 7-day staging soak
- [ ] Tag `v0.1.0` and publish release notes
