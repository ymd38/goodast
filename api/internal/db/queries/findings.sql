-- name: ListFindingsByScan :many
-- スキャン結果レポート（明細）用。重大度の重い順（Critical→Info）、同一 severity 内は
-- 検出順（created_at 昇順）で返す。severity は DB CHECK で 5 値に固定。
SELECT id, template_id, title, severity, url, cwe, remediation, status
FROM findings
WHERE scan_id = $1
ORDER BY
    CASE severity
        WHEN 'Critical' THEN 0
        WHEN 'High' THEN 1
        WHEN 'Medium' THEN 2
        WHEN 'Low' THEN 3
        ELSE 4
    END,
    created_at ASC;
