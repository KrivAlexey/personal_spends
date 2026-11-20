-- shortcut: applied with IF NOT EXISTS at startup, no migration tool; add one
-- on the first schema change this can't express.
CREATE TABLE IF NOT EXISTS uploads (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    sha256          char(64)    NOT NULL UNIQUE,
    filename        text        NOT NULL DEFAULT '',
    bank            text        NOT NULL,
    uploaded_at     timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz NOT NULL DEFAULT now(),
    status          text        NOT NULL DEFAULT 'processing'
                    CHECK (status IN ('processing', 'done', 'failed')),
    imported        integer,
    already_known   integer,
    pending_skipped integer
);
