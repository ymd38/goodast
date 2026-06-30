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
