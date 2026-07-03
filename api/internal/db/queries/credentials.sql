-- name: UpsertScanCredentials :exec
-- session 認証情報を site 単位で作成/更新する（ADR-0003）。auth_mode は session 固定。
-- 「none」は行を作らず不在で表現するため、本クエリは session のみを扱う。
INSERT INTO scan_credentials (site_id, auth_mode, enc_headers)
VALUES ($1, 'session', $2)
ON CONFLICT (site_id) DO UPDATE
SET auth_mode = EXCLUDED.auth_mode,
    enc_headers = EXCLUDED.enc_headers;

-- name: GetScanCredentials :one
-- api はステータス表示のみで復号しないため、機微な enc_headers は取得しない。
SELECT auth_mode, created_at FROM scan_credentials WHERE site_id = $1;

-- name: DeleteScanCredentials :exec
DELETE FROM scan_credentials WHERE site_id = $1;
