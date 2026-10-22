# AGENTS.md

Instructions for coding agents working in this repository. Read this file before
making changes.

## Product

jobtool is a multi-user tech job tool.

A user uploads a CV, answers a few questions, and receives matched tech jobs with
an explainable match percentage. The user picks a digest frequency (daily, every
3 days, weekly), tracks applications, and later gets a tailor-my-CV feature.

The same code must work in two modes:

1. Self-hosted: clone the repo and run `docker compose up`.
2. Hosted service: deployed from the same repo and image definitions.

Do not introduce code paths that only work in one of the two modes.

## Scope

Tech jobs only for now. Role families are config-driven, not hardcoded. The
initial families are:

`backend`, `ai_ml`, `data`, `frontend`, `fullstack`, `devops`, `mobile`,
`security`, `qa`.

## Architecture

- `backend/` is a single Go module producing one binary. Roles `api`, `worker`
  and `scheduler` are selected by flag (`--role`, plus `--migrate` for a
  one-shot migration run).
- `web/` is a Next.js application in TypeScript.
- Postgres with the pgvector extension is the only data store.
- The job queue is Postgres-backed. Do not add Redis, Kafka, or another broker.
- One shared job corpus is ingested once for everyone. Per-user matching runs on
  top of that corpus. Never duplicate job rows per user.

## Scoring rules

Scoring is deterministic and explainable.

1. Apply hard gates first. A gated job is excluded, not scored low.
2. Then compute weighted components:
   - required-skill coverage
   - seniority and years fit
   - role family
   - embedding similarity
   - recency
3. Output is 0 to 100 plus a breakdown, matched skills, and missing skills.

The LLM never produces the score. The score is a pure function of stored data
and config.

## LLM is optional

There must be a baseline that needs no API keys: skill dictionary extraction,
date-range years estimation, and local embeddings. It must work end to end.

An LLM provider interface (Ollama or any OpenAI-compatible endpoint) improves
extraction when configured. Absent configuration, the baseline is used. Absent
configuration must never cause a startup failure or a broken request.

## Conventions

- `net/http` with the Go 1.22 pattern mux. No router library.
- `pgx` for Postgres access.
- `sqlc` for generated queries.
- `goose` for migrations.
- `slog` for structured logging.
- Table-driven tests.
- `httptest` fixtures for external sources.
- No hardcoded keywords, slugs, thresholds or weights in logic. They live in
  config.
- Config comes from environment variables and is validated at startup.
- Minimal code comments.
- No emoji.

## Non-goals

- No LinkedIn scraping.
- No Indeed scraping.
- No auto-apply.
- No scraping of X.

## Rules

- Never commit or push to a remote.
- Never write secrets to files.
- Ask before adding a dependency that is not listed above.
