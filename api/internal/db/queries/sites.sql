-- name: CreateSite :one
INSERT INTO sites (name, base_url, verify_method, verify_token, ownership_verified)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSiteByID :one
SELECT * FROM sites WHERE id = $1;

-- name: GetSiteByName :one
SELECT * FROM sites WHERE name = $1;

-- name: ListSites :many
SELECT * FROM sites ORDER BY created_at DESC;

-- name: MarkSiteVerified :one
UPDATE sites
SET ownership_verified = true
WHERE id = $1
RETURNING *;
