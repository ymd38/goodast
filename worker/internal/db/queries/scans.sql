-- name: StartScan :one
UPDATE scans
SET status = 'running', started_at = now(), engine_version = $2
WHERE id = $1
RETURNING *;

-- name: CompleteScan :one
UPDATE scans
SET status = 'done', finished_at = now(), summary_json = $2
WHERE id = $1
RETURNING *;

-- name: FailScan :one
UPDATE scans
SET status = 'failed', finished_at = now()
WHERE id = $1
RETURNING *;
