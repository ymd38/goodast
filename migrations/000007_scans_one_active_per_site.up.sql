-- 同一サイトで同時に複数のスキャンを実行させない（queued / running は各サイト最大1件）。
-- queued→running は同一行の status 更新なので許容され、done / failed は対象外となり再スキャン可能。
CREATE UNIQUE INDEX scans_one_active_per_site
    ON scans (site_id)
    WHERE status IN ('queued', 'running');
