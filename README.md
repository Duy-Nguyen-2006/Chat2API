# Chat2API

OpenAI-compatible API in front of ChatGPT, written in Go.

Chat2API exposes ChatGPT as a drop-in replacement for the OpenAI REST API so
existing clients (and pipelines built around the OpenAI shape) can keep
working while you route traffic through one or more ChatGPT accounts.

| Capability                | Endpoint                              |
|---------------------------|---------------------------------------|
| Chat completions          | `POST /v1/chat/completions`           |
| Image generation          | `POST /v1/images/generations`         |
| Image edit                | `POST /v1/images/edits`               |
| Anthropic Messages        | `POST /v1/messages`                   |
| Models listing            | `GET  /v1/models`                     |
| Workspaces listing        | `GET  /v1/workspaces`                 |
| Custom GPTs listing       | `GET  /v1/gpts`                       |
| Admin dashboard / API     | `GET  /admin/` and `/admin/api/...`   |

All non-admin endpoints accept either an OpenAI-style `Authorization: Bearer
sk-…` API key issued by the admin API, or — when `DISABLE_AUTH=true` —
unauthenticated traffic.

---

## Features

- **Account pool with round-robin failover.** Multiple ChatGPT accounts
  (loaded from `accounts.json` or seeded from a single access token /
  cookie export) share load; on `401` / `403` / network error the request
  retries on the next account.
- **OAuth refresh + watcher.** Each token's expiry is monitored in the
  background and refreshed via `auth.openai.com/oauth/token`; alias-chain
  mapping prevents in-flight requests from drifting to a dead token.
- **401 eviction with grace.** Accounts that fail are marked invalid and
  removed (or parked, depending on `AUTO_REMOVE_INVALID_ACCOUNTS`) so the
  pool self-heals.
- **Image generation pipeline.** Five-step flow (prepare → upload →
  start SSE → poll → resolve URLs) for `gpt-image-2` /
  `codex-gpt-image-2`, slot-tracked per account so a single account isn't
  flooded with concurrent image jobs.
- **Anthropic Messages adapter.** `POST /v1/messages` transcribes ChatGPT
  output to Anthropic's SSE format so Claude-SDK clients work unchanged.
- **TLS fingerprint impersonation.** Uses Chrome 133 ClientHello + HTTP/2
  fingerprint via `bogdanfinn/tls-client` so requests pass Cloudflare's
  bot checks.
- **Cloudflare Turnstile solver.** Pure-Go port of the bytecode VM that
  resolves the `dx` challenge for `chatgpt.com` request signing.
- **API auth + storage abstraction.** API keys are SHA-256 hashed (constant
  time compare). State is persisted through a pluggable backend — JSON
  (default, atomic write) or SQLite (pure-Go via `modernc.org/sqlite`).
- **Auto-Approve tool calls.** Custom GPT / MCP tool permission prompts
  are auto-accepted (`AUTO_APPROVE_TOOLS=true`).

---

## Quick start

```bash
# 1. Get a token. Easiest: open DevTools on chatgpt.com while logged in,
#    then run in the console:
#      (await (await fetch('/api/auth/session')).json()).accessToken
# 2. Configure
cp .env.example .env
echo "CHATGPT_ACCESS_TOKEN=eyJ…" >> .env
echo "AUTH_KEY=$(openssl rand -hex 32)"          >> .env   # admin master key
# 3. Run
./run.sh
# → listening on http://localhost:8080
```

A `curl` smoke-test:

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer ${AUTH_KEY}"
```

Then issue API keys via the admin API and use them with normal OpenAI
clients.

---

## Configuration

See [`.env.example`](.env.example) for the full list. Highlights:

| Variable                       | Default       | Purpose                                        |
|--------------------------------|---------------|------------------------------------------------|
| `CHATGPT_ACCESS_TOKEN`         | (unset)       | JWT access token; seed account                |
| `COOKIES_FILE`                 | `cookies_1.json` | Fallback cookie export                      |
| `ACCOUNTS_FILE`                | `accounts.json` | Pool persistence                            |
| `REFRESH_ACCOUNT_INTERVAL_MIN` | `5`           | Watcher cadence                               |
| `IMAGE_ACCOUNT_CONCURRENCY`    | `3`           | Per-account in-flight image slots             |
| `AUTH_KEY`                     | (unset)       | Admin master key                              |
| `DISABLE_AUTH`                 | `false`       | Bypass auth middleware (dev only)             |
| `STORAGE_BACKEND`              | `json`        | `json` or `sqlite`                            |
| `STORAGE_DIR`                  | `data`        | Directory for JSON backend                    |
| `SQLITE_PATH`                  | (unset)       | File path for SQLite backend                  |
| `AUTO_APPROVE_TOOLS`           | `true`        | Auto-accept GPT/MCP permission prompts        |
| `PROXY`                        | (unset)       | Global upstream proxy                         |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                          HTTP request                                │
└──────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
                  ┌────────────────────────────┐
                  │   middleware (cors, auth)  │  internal/auth + server
                  └────────────────────────────┘
                                  │
            ┌────────────┬────────┼────────┬────────────────┐
            ▼            ▼        ▼        ▼                ▼
   /v1/chat/...   /v1/images/*  /v1/messages  /v1/models,    /admin/*
                                       gpts,workspaces
            │             │            │              │            │
            └─────────────┴────────────┴──────────────┴────────────┘
                                  │
                                  ▼
                       internal/protocol  (adapters)
                                  │
                                  ▼
                internal/chatgpt.Client (HTTP primitives)
                                  │
                  TLS-impersonating http.Client
                                  │
                                  ▼
                            chatgpt.com
```

**Packages**

| Package                       | Responsibility                                              |
|-------------------------------|-------------------------------------------------------------|
| `cmd/chat2api`                | Entry-point; signal handling                                |
| `internal/config`             | `.env` parsing                                              |
| `internal/httpclient`         | TLS-impersonating client + browser fingerprint headers      |
| `internal/chatgpt`            | Raw ChatGPT HTTP primitives + Turnstile solver              |
| `internal/account`            | Pool, OAuth refresh, watcher, JWT helpers                   |
| `internal/auth`               | API-key issuance, SHA-256 hashing, middleware               |
| `internal/storage`            | Backend interface; JSON and SQLite implementations          |
| `internal/protocol`           | OpenAI / Anthropic / image adapters                         |
| `internal/admin`              | Admin dashboard + CRUD endpoints                            |
| `internal/server`             | Routing, middleware composition, retry loop                 |

---

## Build / test

```bash
go build ./...
go vet ./...
go test ./...
```

Requires Go 1.24+.

---

## License

MIT. See upstream of the original TypeScript codebase for prior history.