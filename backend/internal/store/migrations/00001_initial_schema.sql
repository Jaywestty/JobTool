-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- users and sessions
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text        NOT NULL,
    email_norm    text        NOT NULL GENERATED ALWAYS AS (lower(btrim(email))) STORED,
    password_hash text        NOT NULL,
    display_name  text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_norm_unique UNIQUE (email_norm)
);

CREATE TABLE sessions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash)
);

-- Sessions are looked up by token hash on every authenticated request.
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
-- Lets expired-session cleanup scan only live sessions.
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at) WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- skills (canonical vocabulary + aliases)
-- ---------------------------------------------------------------------------

CREATE TABLE skills (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug       text NOT NULL,
    name       text NOT NULL,
    aliases    text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT skills_slug_unique UNIQUE (slug)
);

-- Alias lookup is the hot path of baseline extraction: one GIN index finds a
-- skill by any alias without a table scan.
CREATE INDEX skills_aliases_gin_idx ON skills USING gin (aliases);

-- ---------------------------------------------------------------------------
-- cvs and profiles
-- ---------------------------------------------------------------------------

CREATE TABLE cvs (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    filename     text        NOT NULL,
    mime_type    text        NOT NULL,
    content_hash text        NOT NULL,
    -- Raw text only. Binary blobs live in object storage, never in Postgres.
    raw_text     text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cvs_user_content_hash_unique UNIQUE (user_id, content_hash)
);

CREATE INDEX cvs_user_id_created_at_idx ON cvs (user_id, created_at DESC);

CREATE TABLE profiles (
    user_id             uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    headline            text        NOT NULL DEFAULT '',
    years_experience    numeric(4,1),
    current_seniority   text,
    preferred_role_family text,
    remote_scope        text        NOT NULL DEFAULT 'any',
    locations           text[]      NOT NULL DEFAULT '{}',
    extraction_source   text        NOT NULL DEFAULT 'baseline',
    embedding_model     text,
    embedding_dim       integer,
    embedding           vector,
    qa_answers          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE profile_skills (
    user_id      uuid   NOT NULL REFERENCES profiles (user_id) ON DELETE CASCADE,
    skill_id     bigint NOT NULL REFERENCES skills (id) ON DELETE CASCADE,
    years        numeric(4,1),
    source       text   NOT NULL DEFAULT 'cv',
    confidence   numeric(4,3) NOT NULL DEFAULT 1.000,
    PRIMARY KEY (user_id, skill_id)
);

-- Reverse lookups: which users have a given skill.
CREATE INDEX profile_skills_skill_id_idx ON profile_skills (skill_id);

-- ---------------------------------------------------------------------------
-- jobs (one shared corpus for all users)
-- ---------------------------------------------------------------------------

CREATE TABLE jobs (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical_url   text        NOT NULL,
    -- Dedupe keys: url_hash catches the same posting from any source;
    -- content_hash catches identical text re-posted or re-crawled.
    url_hash        bytea       NOT NULL,
    content_hash    bytea       NOT NULL,
    source          text        NOT NULL,
    company         text        NOT NULL DEFAULT '',
    title           text        NOT NULL,
    description     text        NOT NULL,
    location        text        NOT NULL DEFAULT '',
    remote_scope    text        NOT NULL DEFAULT 'unknown',
    seniority       text        NOT NULL DEFAULT 'unknown',
    role_family     text,
    salary_min      integer,
    salary_max      integer,
    salary_currency text,
    posted_at       timestamptz,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    is_active       boolean     NOT NULL DEFAULT true,
    CONSTRAINT jobs_url_hash_unique UNIQUE (url_hash)
);

-- A content hash may legitimately repeat across sources; we only need to find
-- the duplicate quickly, so this is a non-unique index.
CREATE INDEX jobs_content_hash_idx ON jobs (content_hash);
-- Matching scans a live, recent slice of the corpus filtered by family.
CREATE INDEX jobs_active_family_posted_idx ON jobs (role_family, posted_at DESC) WHERE is_active;
CREATE INDEX jobs_posted_at_idx ON jobs (posted_at DESC);
CREATE INDEX jobs_first_seen_at_idx ON jobs (first_seen_at DESC);
-- Full-text search over title and description, kept out of the matching path
-- but available for search and manual tools.
CREATE INDEX jobs_fts_idx ON jobs USING gin (
    (to_tsvector('english', title || ' ' || description))
);

CREATE TABLE job_skills (
    job_id    uuid    NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    skill_id  bigint  NOT NULL REFERENCES skills (id) ON DELETE CASCADE,
    must_have boolean NOT NULL DEFAULT false,
    PRIMARY KEY (job_id, skill_id)
);

-- Matching loads all skills for a set of jobs, and reverse lookups (jobs
-- requiring a skill) use this index.
CREATE INDEX job_skills_skill_id_idx ON job_skills (skill_id);
CREATE INDEX job_skills_must_have_idx ON job_skills (skill_id) WHERE must_have;

-- ---------------------------------------------------------------------------
-- embeddings
-- ---------------------------------------------------------------------------

-- The vector column is created without a fixed dimension because the
-- dimension is config-driven: the model name and dimension are recorded on
-- every row, and the application refuses to compare vectors whose dimension
-- does not match the configured one. A fixed vector(1536) column would lock
-- the deployment to one model.
CREATE TABLE job_embeddings (
    job_id     uuid    NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    model      text    NOT NULL,
    dimension  integer NOT NULL CHECK (dimension > 0),
    embedding  vector  NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, model)
);

-- Vector index reasoning:
-- The pgvector HNSW index requires a fixed dimension, so it lives on an
-- expression cast to the configured dimension. The deployment default is 768
-- (JOBTOOL_EMBEDDING_DIM=768). If the operator changes the dimension they must
-- create a matching expression index, which the application logs at startup.
-- HNSW over IVFFlat because the corpus is append-mostly and HNSW gives better
-- recall without a periodic training step on a growing table; cosine ops
-- because embeddings are L2-normalised at write time.
CREATE INDEX job_embeddings_hnsw_cosine_idx
    ON job_embeddings USING hnsw ((embedding::vector(768)) vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
-- Lets the matcher filter to the active model before the ANN scan.
CREATE INDEX job_embeddings_model_idx ON job_embeddings (model, dimension);

-- ---------------------------------------------------------------------------
-- matches
-- ---------------------------------------------------------------------------

CREATE TABLE matches (
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id       uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    score        numeric(5,2) NOT NULL CHECK (score >= 0 AND score <= 100),
    breakdown    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    matched_skills text[]    NOT NULL DEFAULT '{}',
    missing_skills text[]    NOT NULL DEFAULT '{}',
    scoring_version text   NOT NULL,
    computed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);

-- Digest and feed read a user's matches in descending score order.
CREATE INDEX matches_user_score_idx ON matches (user_id, score DESC);
CREATE INDEX matches_job_id_idx ON matches (job_id);

-- ---------------------------------------------------------------------------
-- applications
-- ---------------------------------------------------------------------------

CREATE TABLE applications (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id           uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    status           text        NOT NULL DEFAULT 'saved',
    interview_status text,
    notes            text        NOT NULL DEFAULT '',
    cv_version       uuid        REFERENCES cvs (id) ON DELETE SET NULL,
    applied_at       timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT applications_user_job_unique UNIQUE (user_id, job_id),
    CONSTRAINT applications_status_check CHECK (
        status IN ('saved', 'applied', 'screening', 'interview', 'offer', 'rejected', 'withdrawn')
    )
);

CREATE INDEX applications_user_status_idx ON applications (user_id, status, updated_at DESC);

-- ---------------------------------------------------------------------------
-- feedback (save, apply, dismiss)
-- ---------------------------------------------------------------------------

CREATE TABLE feedback (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id     uuid        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    kind       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT feedback_kind_check CHECK (kind IN ('save', 'apply', 'dismiss')),
    CONSTRAINT feedback_user_job_kind_unique UNIQUE (user_id, job_id, kind)
);

-- The scorer may read feedback as a future signal; this index supports that
-- and the per-user feedback list.
CREATE INDEX feedback_user_created_at_idx ON feedback (user_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- digest preferences
-- ---------------------------------------------------------------------------

CREATE TABLE digest_preferences (
    user_id    uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    frequency  text        NOT NULL,
    timezone   text        NOT NULL DEFAULT 'UTC',
    send_hour  smallint    NOT NULL DEFAULT 8 CHECK (send_hour BETWEEN 0 AND 23),
    channels   text[]      NOT NULL DEFAULT '{email}',
    enabled    boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT digest_frequency_check CHECK (frequency IN ('daily', 'every_3_days', 'weekly'))
);

-- The scheduler scans (timezone, send_hour) once a minute to find due users.
CREATE INDEX digest_preferences_due_idx ON digest_preferences (timezone, send_hour) WHERE enabled;

-- ---------------------------------------------------------------------------
-- source runs
-- ---------------------------------------------------------------------------

CREATE TABLE source_runs (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source         text        NOT NULL,
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz,
    status         text        NOT NULL DEFAULT 'running',
    fetched        integer     NOT NULL DEFAULT 0,
    inserted       integer     NOT NULL DEFAULT 0,
    updated        integer     NOT NULL DEFAULT 0,
    skipped        integer     NOT NULL DEFAULT 0,
    error          text,
    CONSTRAINT source_runs_status_check CHECK (status IN ('running', 'ok', 'partial', 'failed'))
);

-- Recent runs per source drive "last successful run" checks.
CREATE INDEX source_runs_source_started_at_idx ON source_runs (source, started_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS source_runs;
DROP TABLE IF EXISTS digest_preferences;
DROP TABLE IF EXISTS feedback;
DROP TABLE IF EXISTS applications;
DROP TABLE IF EXISTS matches;
DROP INDEX IF EXISTS job_embeddings_hnsw_cosine_idx;
DROP TABLE IF EXISTS job_embeddings;
DROP TABLE IF EXISTS job_skills;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS profile_skills;
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS cvs;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
