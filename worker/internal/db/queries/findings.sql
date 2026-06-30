-- name: InsertFinding :one
INSERT INTO findings (scan_id, template_id, title, severity, url, cwe, remediation)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: DeleteFindingsByScan :exec
-- スキャン再試行（running からの再開）時に、部分保存済みの findings を掃除して
-- 再挿入による重複を防ぐ。スキャン実行直前に呼ぶことで再実行を冪等にする。
DELETE FROM findings WHERE scan_id = $1;
