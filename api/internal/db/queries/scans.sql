-- name: CreateScan :one
INSERT INTO scans (site_id)
VALUES ($1)
RETURNING *;

-- name: GetScan :one
SELECT * FROM scans WHERE id = $1;

-- name: ListScansBySite :many
SELECT * FROM scans
WHERE site_id = $1
ORDER BY created_at DESC;

-- name: ListDoneScanSummaries :many
-- ダッシュボードのスコア時系列用。完了かつ summary_json を持つスキャンのみを
-- 日付昇順（折れ線 左→右）で返す。スコアは呼び出し側（report）で summary から算出する。
SELECT id, created_at, finished_at, summary_json
FROM scans
WHERE site_id = $1 AND status = 'done' AND summary_json IS NOT NULL
ORDER BY created_at ASC;
