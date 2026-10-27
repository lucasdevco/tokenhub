-- name: BeginCaptureWrite :one
INSERT OR REPLACE INTO pending(source, name, ready) VALUES (?, ?, 0) RETURNING id;
-- name: EndCaptureWrite :exec
UPDATE pending SET ready = 1 WHERE id = ?;
-- name: RecoverCaptureWrites :exec
UPDATE pending SET ready = 1 WHERE ready = 0 AND source = ?;
-- name: PendingFiles :many
SELECT id, name FROM pending WHERE ready = 1 AND source = sqlc.arg(source) ORDER BY id DESC LIMIT sqlc.arg(page_size);
-- name: AckCaptureWrite :exec
DELETE FROM pending WHERE id = ?;
-- name: GetSetting :one
SELECT value FROM settings WHERE name = ?;
-- name: PutSetting :exec
INSERT INTO settings(name, value) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET value = excluded.value;
-- name: GetRecord :one
SELECT * FROM records WHERE key = sqlc.arg(key) AND source = sqlc.arg(source);
-- name: GetMetadata :one
SELECT * FROM records WHERE metadata_file = sqlc.arg(metadata_file) AND source = sqlc.arg(source);
-- name: PutRecord :exec
INSERT INTO records(source,metadata_file,key,started,state,method,capture_error,failed,search_text,summary,size,modified,watch,seen)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(source,metadata_file) DO UPDATE SET key=excluded.key, started=excluded.started,
state=excluded.state, method=excluded.method, capture_error=excluded.capture_error,
failed=excluded.failed, search_text=excluded.search_text, summary=excluded.summary,
size=excluded.size, modified=excluded.modified, watch=excluded.watch, seen=excluded.seen;
-- name: TouchRecord :exec
UPDATE records SET seen = ? WHERE metadata_file = sqlc.arg(metadata_file) AND source = sqlc.arg(source);
-- name: DeleteRecord :exec
DELETE FROM records WHERE metadata_file = sqlc.arg(metadata_file) AND source = sqlc.arg(source);
-- name: ActiveFiles :many
SELECT metadata_file FROM records WHERE source=sqlc.arg(source) AND watch = 1 AND metadata_file > sqlc.arg(after_name)
ORDER BY metadata_file LIMIT sqlc.arg(page_size);
-- name: GetFile :one
SELECT * FROM files WHERE name=sqlc.arg(name) AND source=sqlc.arg(source);
-- name: PutFile :exec
INSERT INTO files(source,name,owner,size,modified,problem,complete,seen) VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(source,name) DO UPDATE SET owner=excluded.owner,size=excluded.size,modified=excluded.modified,
problem=excluded.problem,complete=excluded.complete,seen=excluded.seen;
-- name: TouchFile :exec
UPDATE files SET seen=? WHERE name=sqlc.arg(name) AND source=sqlc.arg(source);
-- name: DeleteFile :exec
DELETE FROM files WHERE name=sqlc.arg(name) AND source=sqlc.arg(source);
-- name: StaleFiles :many
SELECT name, owner FROM files WHERE source=sqlc.arg(source) AND seen <> sqlc.arg(generation) AND name > sqlc.arg(after_name) ORDER BY name LIMIT sqlc.arg(page_size);
-- name: ListRecords :many
SELECT key,started,summary FROM records
WHERE source=sqlc.arg(source) AND (started,key) < (CAST(sqlc.arg(before_started) AS INTEGER),CAST(sqlc.arg(before_key) AS TEXT))
AND (CAST(sqlc.arg(state) AS TEXT) = '' OR state = CAST(sqlc.arg(state) AS TEXT) OR (CAST(sqlc.arg(state) AS TEXT) = 'capture_error' AND capture_error = 1))
AND (CAST(sqlc.arg(method) AS TEXT) = '' OR method = CAST(sqlc.arg(method) AS TEXT))
AND (CAST(sqlc.arg(search) AS TEXT) = '' OR instr(search_text,CAST(sqlc.arg(search) AS TEXT)) > 0)
ORDER BY started DESC,key DESC LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);
-- name: ListFiles :many
SELECT * FROM files WHERE source=sqlc.arg(source) AND name < sqlc.arg(before_name)
AND (CAST(sqlc.arg(search) AS TEXT) = '' OR instr(lower(name),CAST(sqlc.arg(search) AS TEXT)) > 0)
ORDER BY name DESC LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);
-- name: SummaryTotals :many
SELECT * FROM totals WHERE n > 0 AND source=?;
-- name: CountSearch :one
SELECT COUNT(*) FROM records WHERE source=sqlc.arg(source) AND instr(search_text,CAST(sqlc.arg(search) AS TEXT)) > 0
AND (CAST(sqlc.arg(state) AS TEXT) = '' OR state = CAST(sqlc.arg(state) AS TEXT) OR (CAST(sqlc.arg(state) AS TEXT) = 'capture_error' AND capture_error = 1))
AND (CAST(sqlc.arg(method) AS TEXT) = '' OR method = CAST(sqlc.arg(method) AS TEXT));
-- name: CountFileSearch :one
SELECT COUNT(*) FROM files WHERE source=sqlc.arg(source) AND instr(lower(name),CAST(sqlc.arg(search) AS TEXT)) > 0;
-- name: Histogram :many
SELECT bucket, CAST(SUM(n) AS INTEGER) AS n FROM buckets
WHERE source=sqlc.arg(source) AND bucket >= sqlc.arg(first_bucket) AND bucket <= sqlc.arg(last_bucket)
AND (CAST(sqlc.arg(state) AS TEXT) = '' OR state = CAST(sqlc.arg(state) AS TEXT) OR (CAST(sqlc.arg(state) AS TEXT) = 'capture_error' AND capture_error = 1))
AND (CAST(sqlc.arg(method) AS TEXT) = '' OR method = CAST(sqlc.arg(method) AS TEXT))
GROUP BY bucket;
-- name: SearchHistogram :many
SELECT CAST(started / 120000000000 AS INTEGER) AS bucket, COUNT(*) AS n FROM records
WHERE source=sqlc.arg(source) AND started >= sqlc.arg(start_time) AND started <= sqlc.arg(end_time)
AND instr(search_text,CAST(sqlc.arg(search) AS TEXT)) > 0
AND (CAST(sqlc.arg(state) AS TEXT) = '' OR state = CAST(sqlc.arg(state) AS TEXT) OR (CAST(sqlc.arg(state) AS TEXT) = 'capture_error' AND capture_error = 1))
AND (CAST(sqlc.arg(method) AS TEXT) = '' OR method = CAST(sqlc.arg(method) AS TEXT))
GROUP BY bucket;

-- name: OrphanFiles :exec
UPDATE files SET owner='',complete=0 WHERE owner=sqlc.arg(owner) AND source=sqlc.arg(source);
