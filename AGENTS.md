# gmbox Agent Guide

## Scope

- This file applies to the entire repository at `/home/openchamber/workspaces/gmbox`.
- No repo-local Cursor rules were found in `.cursor/rules/` or `.cursorrules`.
- No repo-local Copilot rules were found in `.github/copilot-instructions.md`.

## Stack Overview

- Backend: Go `1.26.1`, Gin, GORM, sqlite/postgres/mysql support.
- Frontend: Vue `3`, TypeScript, Vite, Quasar.
- Build coupling: the frontend build output is embedded into the Go server binary.
- Important output path: `web/` builds into `frontend/dist/`, and the backend expects that directory for full builds.

## Working Principles

- Prefer the smallest correct change.
- Keep behavior aligned with existing screens and endpoints before inventing new patterns.
- Do not add new frameworks, linters, or formatters unless the user explicitly asks.
- Comments in this repository are written in Chinese and should explain why, not restate obvious code.

## Source Of Truth

- Commands come from `Makefile`, `package.json`, and `README.md`.
- Style comes from existing code under `internal/` and `web/src/`; prefer nearby patterns over generic advice.

## Build Commands

- Install frontend dependencies: `make deps`
- Direct equivalent: `npm install`
- Build frontend only: `make web-build`
- Direct equivalent: `npm run build`
- Build backend only: `make server-build`
- Direct equivalent: `go build -o ./gmbox ./cmd/server`
- Full production build: `make build`
- Full build order matters: frontend must be built before backend so Go embed can include static assets.
- Run server locally: `make run`
- Direct equivalent: `go run ./cmd/server`
- Clean generated artifacts: `make clean`

## Test Commands

- Run all Go tests: `make test`
- Direct equivalent: `go test ./...`
- Run tests for one package: `go test ./internal/httpapi`
- Run one named test in one package: `go test ./internal/httpapi -run TestRedactQueryForLog -v`
- Another example: `go test ./internal/mail -run TestShouldRetryOAuthSync -v`
- Run multiple tests by regex: `go test ./internal/mail -run 'TestShouldRetryOAuthSync|TestSyncResultZeroValue' -v`
- Disable test cache when needed: `go test ./internal/mail -run TestShouldRetryOAuthSync -count=1 -v`
- Run all tests in one package with verbose output: `go test ./internal/mail -v`

## Frontend Verification

- There is currently no frontend unit test script in `package.json`.
- There is currently no repo-local ESLint or Prettier config.
- The most reliable frontend validation command is: `npm run build`
- Use `make build` after cross-stack changes because it validates the embed pipeline end to end.

## Lint And Formatting Reality

- No dedicated lint command is configured in `Makefile` or `package.json`.
- Do not invent `npm run lint` or `make lint` in automation unless you also add and document them.
- For Go formatting, use `gofmt -w <file>` on changed Go files.
- If `goimports` is available locally, it is safe to use, but it is not configured as a required repo command.
- Before finishing a non-trivial change, run at least the relevant Go tests and `npm run build` when frontend code changed.

## Repository Layout

- `cmd/server`: backend entrypoint.
- `internal/`: backend packages including routing, runtime, mail, auth, config, and models.
- `web/src`: Vue application source.
- `frontend/dist`: generated frontend assets consumed by Go embed.

## Go Style

- Always let `gofmt` shape whitespace and alignment.
- Keep imports grouped as: standard library, third-party, local module imports.
- Use package aliases only when they remove ambiguity or shorten noisy package names meaningfully.
- Exported names use PascalCase.
- Unexported names use lowerCamelCase.
- Constructor-style functions use `NewX`, for example `NewRouter`, `NewService`, `NewSyncer`.
- Keep functions focused; prefer a helper when a block has a separate responsibility or needs reuse.

## Go Error Handling

- Return early on invalid input or failed dependencies.
- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Use `errors.Is` when checking sentinel or library errors such as `gorm.ErrRecordNotFound`.
- Use user-facing Chinese error messages in HTTP JSON responses.
- Use structured logging via `slog` for operational events.
- Prefer log fields like `"err", err` and other structured key/value pairs over concatenated strings.

## Go HTTP Handler Conventions

- Gin handlers usually validate JSON with `ShouldBindJSON` and return `400` on bad input.
- HTTP JSON error payloads typically use `gin.H{"message": "..."}`.
- Authentication-protected routes are grouped under `/api` and wired through middleware.
- Validate request constraints before hitting the database where practical.
- Keep handler messages in Chinese to match the existing UI and API behavior.

## Go Data Model Conventions

- GORM models embed `utilsdb.Model`.
- JSON tags use snake_case.
- Database field tags are explicit about size, indexes, uniqueness, and nullability.
- Keep model field names descriptive and aligned with API payload names where possible.

## Go Testing Conventions

- Test files live beside the implementation as `*_test.go`.
- Test names use `TestXxx` with behavior-oriented wording.
- Table-driven tests are used when multiple cases share one behavior.
- Small local test helpers are preferred over heavy fixtures when only a tiny abstraction is needed.

## Frontend Style

- Use Vue SFCs with `<script setup lang="ts">`.
- Prefer composition API primitives already used in the repo: `ref`, `reactive`, `computed`, `watch`, `onMounted`, `onBeforeUnmount`.
- Keep imports grouped logically: Vue/Vue Router, components, API/types, local utilities.
- Use the `@` alias for `web/src` imports.
- Use PascalCase for component files and imported components.
- Use lowerCamelCase for variables and functions.
- Use `type` or `interface` for data shapes; follow surrounding code rather than forcing one universally.
- Keep UI text and comments in Chinese to match the rest of the app.

## Frontend Data And Requests

- Centralize fetch behavior through `request<T>()` from `web/src/api.ts` unless there is a strong reason not to.
- Assume authenticated requests need `credentials: 'include'`.
- Parse and present backend `message` fields to users instead of replacing them with generic English text.
- Catch async request failures and map them with `err instanceof Error ? err.message : '默认文案'`.
- Prefer explicit form reset helpers such as `createDefaultForm()` and `resetForm()` when state is reused.

## Frontend Formatting And Templates

- Match the existing no-semicolon TypeScript style.
- Match the existing single-quote string style in TypeScript.
- Keep templates declarative; move non-trivial branching or shaping logic into script helpers.
- Reuse existing Quasar patterns already present in nearby views.

## Change Safety Rules

- Do not break the frontend-to-backend payload contract without updating both sides.
- Because the server embeds `frontend/dist`, remember that backend-only builds can depend on prior frontend output.
- If you change routes, API payloads, or model fields, inspect all direct consumers before finishing.
- Preserve unrelated user changes already present in the worktree.

## Practical Completion Checklist

- Format changed Go files with `gofmt`.
- Run targeted Go tests for the touched package.
- Run `go test ./...` for meaningful backend changes.
- Run `npm run build` for any frontend change.
- Run `make build` for full-stack or embed-sensitive changes.
