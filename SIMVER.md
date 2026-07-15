# SIMVER — Versioning and Tagging Rules

This document defines the versioning policy for the `emailer` project. All
git tags MUST follow the rules below. This is policy only — no code is
written here.

## 1. Scheme

Versions follow [Semantic Versioning 2.0.0](https://semver.org):

```text
vMAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
```

- `MAJOR`, `MINOR`, `PATCH` are non-negative integers, never zero-padded.
- The `v` prefix is mandatory on every tag.
- Examples: `v0.1.0`, `v1.0.0`, `v1.4.2`, `v2.0.0-rc.1`, `v1.2.0-rc.3`.

## 2. Component Bump Rules

| Bump | When |
| --- | --- |
| `MAJOR` | Any backwards-incompatible change: config schema break, IMAP flag rename, CLI flag removal/semantic change, digest format contract change, store migration that is not auto-upgradable. |
| `MINOR` | Backwards-compatible feature addition: new provider, new digest section, new optional config field with a safe default, new notification channel. |
| `PATCH` | Backwards-compatible fix: bug fix, error-handling improvement, doc fix, dependency security patch with no behavior change. |

No component is ever decremented. After a `MAJOR` bump, `MINOR` and `PATCH`
reset to `0`. After a `MINOR` bump, `PATCH` resets to `0`.

## 3. Pre-Releases

- Pre-release tags use the `-rc.N` suffix, where `N` starts at `1` and
  increments per candidate: `v1.0.0-rc.1`, `v1.0.0-rc.2`.
- A pre-release is lower in precedence than the associated release
  (`v1.0.0-rc.1` < `v1.0.0`).
- Pre-releases are for staging/QA only. Production runs should pin a full
  release tag.
- Build metadata (`+BUILD`) is permitted but ignored for precedence and
  MUST NOT carry secrets.

## 4. Relationship to Commit Messages

The repo uses Conventional Commits (`type(scope): subject`). Tag bumps are
derived from merged content, not individual commits, because the project
squash-merges:

- `feat(...)` present since last release → `MINOR`.
- `fix(...)` / `perf(...)` present since last release → `PATCH`.
- `BREAKING CHANGE:` footer or `!` after type/scope → `MAJOR`.
- `chore`, `docs`, `test`, `refactor`, `style` alone never bump `MAJOR`/
  `MINOR`; they may accompany a `PATCH` if paired with a fix.

## 5. Tagging Procedure

1. Ensure `main` is green (CI: lint, test, build) and `planning.md` Phase
   "Hardening and Final Audit" is complete for the release.
2. Decide the next version from §2/§4.
3. Tag the exact commit: `git tag -s vX.Y.Z -m "emailer vX.Y.Z"`.
   Signed tags (`-s`) are required for release tags.
4. For a release candidate, tag `vX.Y.Z-rc.N` first and run staging for the
   required soak period (see `planning.md`) before cutting the final tag.
5. Push the tag: `git push origin vX.Y.Z`.

## 6. Forbidden Patterns

- No `latest`, `stable`, `current`, or date-based tags as version identifiers.
- No omitting the `v` prefix.
- No reusing or moving an already-pushed tag.
- No `MAJOR` `0` → promoted to `1` except for the explicit `1.0.0` stability
  milestone (first production-ready release).
- No secrets or environment specifics in tag names or messages.

## 7. Current Version

The active release line is tracked by the latest `v*` tag in the repository.
`planning.md` Phase "Release" records the next intended tag
(e.g., `v0.1.0-rc.1` → `v0.1.0`).
