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

## Tests

```sh
cd backend
go test ./...
```

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
  internal/llm/         optional provider interface plus no-key baseline
  internal/platform/    config, logging, HTTP, runner
web/
  app/                  Next.js app router pages
  lib/api.ts            backend client
```

## Rules for contributors

See `AGENTS.md`. In short: no LinkedIn/Indeed scraping, no auto-apply, no
committing secrets, and ask before adding dependencies.
