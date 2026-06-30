-- 状態遷移ガード: WHERE に現在の status を含め、queued→running→done/failed の
-- ライフサイクルのみを許可する。不正な遷移は 0 行更新となり :one では ErrNoRows を
-- 返すため、呼び出し側で「既に遷移済み／不正遷移」として扱える。

-- name: GetScan :one
SELECT * FROM scans WHERE id = $1;

-- name: GetScanTarget :one
-- worker が scan_id からスキャン対象（site）情報をロードする。実スキャン前の
-- defense-in-depth 所有確認（ADR-0004）と、スキャン投入先 URL の取得に用いる。
SELECT
    sc.site_id            AS site_id,
    sc.status             AS status,
    s.base_url            AS base_url,
    s.ownership_verified  AS ownership_verified
FROM scans sc
JOIN sites s ON s.id = sc.site_id
WHERE sc.id = $1;

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
