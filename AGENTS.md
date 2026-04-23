# AGENTS.md

## Overview

This repository is `new-api`, an AI API gateway/proxy written in Go with a React/Vite admin frontend. It unifies many upstream AI providers behind a single API surface and includes user management, billing, routing, quota, auth, and dashboard capabilities.

This file is for coding agents and collaborators working inside this repo. Prefer the existing architecture and conventions over introducing new patterns.

## High-Level Structure

Backend is organized in a layered style:

`router -> controller -> service -> model`

Key directories:

- `main.go`: process entrypoint, resource initialization, task startup, Gin server bootstrap, embedded frontend serving.
- `router/`: route registration.
- `controller/`: HTTP handlers and request orchestration.
- `service/`: business logic and background task coordination.
- `model/`: GORM models, migrations, DB init, cache sync.
- `relay/`: provider relay handlers and format conversion.
- `relay/channel/`: provider-specific adapters and channel implementations.
- `middleware/`: auth, rate limits, i18n, request logging, recovery, distribution.
- `common/`: shared utilities such as env, crypto, Redis, JSON wrappers, rate limiting, monitoring.
- `dto/`: request/response DTOs.
- `setting/`: runtime configuration modules.
- `constant/`, `types/`: constants and shared types.
- `oauth/`: OAuth provider implementations.
- `i18n/`: backend translations.
- `web/`: React 18 + Vite frontend.
- `electron/`: Electron packaging assets.

## Stack

- Backend: Go `1.25.1`, Gin, GORM
- Databases: SQLite, MySQL, PostgreSQL
- Cache: Redis plus in-memory cache
- Frontend: React 18, Vite, Semi UI
- Frontend package manager: `bun` preferred

## Important Repo Facts

- Go module path is `github.com/QuantumNous/new-api`. Do not rename it.
- The frontend is built from `web/` and embedded by `main.go` from `web/dist`.
- There is no top-level `Makefile`; use direct Go/Bun commands.
- Docker-based local startup is documented in `docker-compose.yml`.
- GitHub Actions live under `.github/workflows/`.

## Non-Negotiable Rules

### 1. Preserve Project Identity

Do not modify, remove, rename, or replace protected project identity references related to:

- `new-api`
- `QuantumNous`

This includes branding, module/import paths, titles, metadata, comments, docs, container image names, and attribution text unless the user explicitly requests an allowed addition that does not replace existing identity.

### 2. Use `common/json.go` for JSON Operations

In business code, do not directly call `encoding/json` marshal/unmarshal helpers. Use:

- `common.Marshal`
- `common.Unmarshal`
- `common.UnmarshalJsonStr`
- `common.DecodeJson`
- `common.GetJsonType`

Using `json.RawMessage` or other `encoding/json` types is fine. The restriction is on marshal/unmarshal/decoder calls.

### 3. Keep Database Code Cross-DB Compatible

All DB logic must work with SQLite, MySQL, and PostgreSQL.

Prefer GORM abstractions first. If raw SQL is unavoidable:

- Use the quoting helpers from [`model/main.go`](/workspaces/new-api/model/main.go): `commonGroupCol`, `commonKeyCol`
- Use `commonTrueVal` and `commonFalseVal` for boolean SQL literals
- Branch with `common.UsingSQLite`, `common.UsingMySQL`, `common.UsingPostgreSQL` only when necessary

Avoid database-specific SQL unless there is a clear fallback for all supported engines.

### 4. Preserve Explicit Zero Values in Upstream Relay DTOs

For request DTOs that are parsed from client JSON and then re-marshaled upstream, optional scalar fields must use pointer types with `omitempty`.

Use:

- `*int`, `*uint`, `*float64`, `*bool`

Do not use non-pointer scalar fields with `omitempty` for optional upstream request parameters, or explicit `0` / `false` values will be dropped.

### 5. New Channel/Provider Work Must Respect Stream Options

When adding a new relay channel/provider:

- confirm whether upstream supports `StreamOptions`
- if it does, add the channel to `streamSupportedChannels`
- keep conversions consistent with existing relay DTO zero-value behavior

## Backend Conventions

- Follow the existing layered architecture. Keep HTTP parsing in `controller/`, business logic in `service/`, and persistence in `model/`.
- Reuse existing constants/types before creating new ones.
- Prefer extending existing relay/provider patterns instead of inventing new adapter shapes.
- Background tasks and startup wiring generally belong near `main.go` or existing service/controller task registration points.
- DB migrations belong in the `model/` layer and must be safe for SQLite/MySQL/PostgreSQL.

Useful files:

- [`main.go`](/workspaces/new-api/main.go)
- [`common/json.go`](/workspaces/new-api/common/json.go)
- [`model/main.go`](/workspaces/new-api/model/main.go)

## Frontend Conventions

- Use `bun` in `web/` for install/build/lint/i18n workflows.
- Keep UI aligned with the existing Semi Design based frontend.
- Frontend i18n uses `react-i18next`; translation files live in `web/src/i18n/locales/`.
- Translation keys are Chinese source strings in flat JSON files.
- When adding user-facing text, update translations instead of hardcoding a single language.

Useful files:

- [`web/package.json`](/workspaces/new-api/web/package.json)
- [`web/vite.config.js`](/workspaces/new-api/web/vite.config.js)
- [`web/src/i18n/i18n.js`](/workspaces/new-api/web/src/i18n/i18n.js)

## Internationalization

Backend:

- location: `i18n/`
- languages: `en`, `zh`

Frontend:

- location: `web/src/i18n/`
- languages currently present: `zh-CN`, `zh-TW`, `en`, `fr`, `ru`, `ja`, `vi`
- extraction/sync/lint scripts are defined in `web/package.json`

## Common Commands

Backend from repo root:

```bash
go run .
go test ./...
```

Frontend from `web/`:

```bash
bun install
bun run dev
bun run build
bun run lint
bun run eslint
bun run i18n:extract
bun run i18n:sync
bun run i18n:lint
```

Docker local stack:

```bash
docker-compose up -d
```

## Editing Guidance

- Make the smallest coherent change that fits existing patterns.
- Do not refactor unrelated areas while solving a focused task.
- If you touch request DTOs, double-check zero-value semantics.
- If you touch SQL or migrations, think through SQLite/MySQL/PostgreSQL behavior explicitly.
- If you add UI text, update frontend locale files.
- If you add backend user-facing text, check whether backend i18n is needed.
- Keep comments brief and only where they add real value.

## Validation Expectations

Run the narrowest useful validation for the files you changed. Typical checks:

- Go changes: `go test ./...` or targeted package tests
- Frontend changes: `cd web && bun run build`
- i18n changes: `cd web && bun run i18n:lint`

If a full validation is too expensive, run targeted checks and say what was not run.

## Agent Workflow

When working in this repo:

1. Read the affected layer before editing.
2. Match nearby naming, error handling, and file layout.
3. Preserve protected project identifiers.
4. Validate the specific area you changed.
5. Summarize assumptions if you had to make any.

## Notes

- `docker-compose.yml` defaults to PostgreSQL + Redis, with MySQL example commented out.
- `main.go` starts recurring jobs and embeds the built frontend, so changes that affect startup should be reviewed carefully.
- The repo is cleanly structured already; prefer consistency over broad cleanup.
