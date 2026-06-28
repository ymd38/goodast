-- 状態遷移ガード: WHERE に現在の status を含め、queued→running→done/failed の
-- ライフサイクルのみを許可する。不正な遷移は 0 行更新となり :one では ErrNoRows を
-- 返すため、呼び出し側で「既に遷移済み／不正遷移」として扱える。

-- name: StartScan :one
UPDATE scans
SET status = 'running', started_at = now(), engine_version = $2
WHERE id = $1 AND status = 'queued'
RETURNING *;

-- name: CompleteScan :one
UPDATE scans
SET status = 'done', finished_at = now(), summary_json = $2
WHERE id = $1 AND status = 'running'
RETURNING *;

-- name: FailScan :one
UPDATE scans
SET status = 'failed', finished_at = now()
WHERE id = $1 AND status IN ('queued', 'running')
RETURNING *;
