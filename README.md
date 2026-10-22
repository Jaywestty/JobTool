# jobtool

A multi-user tech job tool. Upload a CV, answer a few questions, and get tech
jobs matched with an explainable percentage. Pick a digest frequency, track
applications, and later tailor your CV.

Tech jobs only for now. Role families are config-driven: backend, ai_ml, data,
frontend, fullstack, devops, mobile, security, qa.

## Stack

- `backend/` one Go binary with roles `api`, `worker`, `scheduler` (`--role`).
- `web/` Next.js in TypeScript.
- Postgres with pgvector. The job queue is Postgres-backed.
- One shared job corpus ingested once for everyone, per-user matching on top.

Scoring is deterministic. Hard gates first, then weighted components, output 0 to
100 with a breakdown. The LLM never produces the score and is optional: the
baseline needs no API keys.

## Run the stack

The whole stack runs with `docker compose up`. Copy the env template first:

```
copy .env.example .env
```

Then edit `.env` and set a real `POSTGRES_PASSWORD`.

### Windows (cmd)

```cmd
cd "C:\path\to\jobtool-app"
copy .env.example .env
notepad .env
docker compose up --build
```

Open `http://localhost:3000` for the web app and `http://localhost:8080/health`
for the backend. Stop with `Ctrl+C`, then `docker compose down`. Add `-v` to
`down` to also delete the database volume.

### Linux

```sh
cd /path/to/jobtool-app
cp .env.example .env
$EDITOR .env
docker compose up --build
```

Same URLs. Stop with `Ctrl+C` and `docker compose down` (add `-v` to wipe data).

## Verify

```sh
curl http://localhost:8080/health
# {"status":"ok","checks":{"postgres":"ok"}}

curl http://localhost:3000/
```

Migrations run automatically in the `migrate` service before the API starts. To
run them by hand:

```sh
docker compose run --rm migrate
```

## Ingestion

Job ingestion ports the source connectors from the earlier Python prototype. A
connector returns normalized rows; a shared pipeline applies the tech gate, the
English check, remote-scope classification and cross-source dedupe, then writes
to Postgres.

Sources: Greenhouse, Lever and Ashby (slugs from config, dead slugs probed at
startup), RemoteOK, Remotive, Himalayas, Jobicy, We Work Remotely, Arbeitnow,
Working Nomads, python.org jobs and Hacker News "Who is hiring?". Every source
is registered from `backend/config/ingest.yaml`; nothing is hardcoded.

- Polite HTTP client: identifying User-Agent, timeouts, per-host spacing,
  retries only on timeouts, connection errors, 429 and 5xx (never 400/401/403/404),
  and a robots.txt check available to crawler-style sources.
- One timestamp normalizer: epoch seconds, epoch milliseconds, ISO-8601 and
  RFC-822 all become UTC.
- Tech gate: role families and keywords in `ingest.yaml`, whole-word matching,
  title first with a description-head fallback, plus an English-language check.
- Remote scope: worldwide, restricted or unknown from config term lists and
  regexes. CET and UK timezone requirements are not treated as restricted.
- Dedupe: canonical URL hash and content hash, so a posting seen twice (or from
  two sources) is stored once. Re-running a source refreshes rows, never
  duplicates them.

Scheduling is Postgres-backed. The scheduler enqueues one ingest task per source
when its interval has elapsed; the worker drains the queue. A per-source
advisory lock prevents the trigger and the scheduler racing on the same source.

Run every source once by hand:

```sh
curl -X POST http://localhost:8080/api/ingest/run
```

Run one source, or list the registered sources:

```sh
curl -X POST http://localhost:8080/api/ingest/run -H "Content-Type: application/json" -d "{\"source\":\"remoteok\"}"
curl http://localhost:8080/api/ingest/sources
```

If `JOBTOOL_INGEST_TRIGGER_TOKEN` is set, the run endpoint requires
`Authorization: Bearer <token>`. Each run is recorded in `source_runs` with
`fetched`, `kept`, `inserted`, `updated` and `error`:

```sh
docker compose exec postgres psql -U jobtool -d jobtool \
  -c "SELECT source, status, fetched, kept, inserted, updated FROM source_runs ORDER BY id DESC LIMIT 20;"
```

## Tests

```sh
cd backend
go test ./...
```

The ingestion package has httptest fixtures for every connector plus tests for
the gate, remote scope, timestamp normalization and dedupe. No network is used
in tests.

## Configuration

All configuration is environment variables, validated at startup. See
`.env.example` for every variable with documentation. Nothing is silently
defaulted except values marked with a default in that file.

Changing `JOBTOOL_EMBEDDING_DIM` requires a matching vector index; see the
comment in `backend/internal/store/migrations/00001_initial_schema.sql`.

## Layout

```
backend/
  cmd/jobtool/          entry point, role selection
  internal/auth/        sessions and password hashing
  internal/cv/          CV text hashing, baseline years from date ranges
  internal/jobs/        URL canonicalisation, dedupe keys, Postgres queue
  internal/sources/     job feeds (Greenhouse), httptest fixtures
  internal/scoring/     deterministic gates and weighted score
  internal/digest/      digest frequency and due-time logic
  internal/store/       pgx pool, embedded goose migrations
  internal/ingest/      source connectors, HTTP client, gates, dedupe, pipeline
  internal/llm/         optional provider interface plus no-key baseline
  internal/platform/    config, logging, HTTP, runner
  config/ingest.yaml    role families, gates, remote scope, source intervals
web/
  app/                  Next.js app router pages
  lib/api.ts            backend client
```

## Rules for contributors

See `AGENTS.md`. In short: no LinkedIn/Indeed scraping, no auto-apply, no
committing secrets, and ask before adding dependencies.
