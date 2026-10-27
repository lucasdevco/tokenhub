CREATE TABLE pending (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    name TEXT NOT NULL,
    ready INTEGER NOT NULL DEFAULT 0,
    UNIQUE(source, name)
);
CREATE INDEX pending_ready ON pending(source, ready, id DESC);
CREATE TABLE settings (name TEXT PRIMARY KEY NOT NULL, value INTEGER NOT NULL);
CREATE TABLE records (
    source TEXT NOT NULL,
    metadata_file TEXT NOT NULL,
    key TEXT NOT NULL,
    started INTEGER NOT NULL,
    state TEXT NOT NULL,
    method TEXT NOT NULL,
    capture_error INTEGER NOT NULL,
    failed INTEGER NOT NULL,
    search_text TEXT NOT NULL,
    summary BLOB NOT NULL,
    size INTEGER NOT NULL,
    modified INTEGER NOT NULL,
    watch INTEGER NOT NULL,
    seen INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(source, metadata_file),
    UNIQUE(source, key)
);
CREATE INDEX records_time ON records(source, started DESC, key DESC);
CREATE INDEX records_watch ON records(source, metadata_file) WHERE watch = 1;
CREATE TABLE files (
    source TEXT NOT NULL,
    name TEXT NOT NULL,
    owner TEXT NOT NULL,
    size INTEGER NOT NULL,
    modified INTEGER NOT NULL,
    problem TEXT NOT NULL,
    complete INTEGER NOT NULL,
    seen INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(source, name)
);
CREATE INDEX files_owner ON files(source, owner);
CREATE TABLE totals (
    source TEXT NOT NULL,
    state TEXT NOT NULL, method TEXT NOT NULL,
    capture_error INTEGER NOT NULL, failed INTEGER NOT NULL,
    n INTEGER NOT NULL,
    PRIMARY KEY(source, state, method, capture_error, failed)
);
CREATE TABLE buckets (
    source TEXT NOT NULL,
    bucket INTEGER NOT NULL, state TEXT NOT NULL, method TEXT NOT NULL,
    capture_error INTEGER NOT NULL, failed INTEGER NOT NULL, n INTEGER NOT NULL,
    PRIMARY KEY(source, bucket, state, method, capture_error, failed)
);
