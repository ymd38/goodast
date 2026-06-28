-- name: InsertFinding :one
INSERT INTO findings (scan_id, template_id, title, severity, url, cwe, remediation)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;
