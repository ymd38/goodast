-- name: GetScanCredentials :one
-- worker が scan_id から当該 site の暗号化認証情報をロードする（復号して注入・ADR-0003）。
-- 認証情報が未設定の scan は ErrNoRows として返り、未認証スキャンとして扱う。
SELECT cred.auth_mode, cred.enc_headers
FROM scans sc
JOIN scan_credentials cred ON cred.site_id = sc.site_id
WHERE sc.id = $1;
