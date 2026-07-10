-- サイトを「ドメイン+ポート（origin）」で一意にする。同一 origin の重複登録を防ぎ、
-- サイト管理・スキャン履歴を1サイトに一元化する。
--
-- origin の正規化ルールは api の target.CanonicalOrigin と一致させる:
--   - スキーム除去・パス除去して host[:port] を得る
--   - ポート省略時はスキーム既定（https=443 / それ以外=80）を補う
--   - ホストは小文字化し、ループバック別名（127.0.0.1 / ::1）は localhost に畳み込む
-- ※アプリ層（target.CanonicalOrigin）が今後の唯一の正。本 backfill は既存行を
--   一意化するための best-effort（既存データは localhost / 通常ホストのみを想定）。

-- 1. origin 列を追加（まずは nullable で backfill する）。
ALTER TABLE sites ADD COLUMN origin text;

-- 2. 既存の重複（同一 base_url）を最古行へ一元化する。
--    スキャンは最古サイトへ付け替え、重複サイトの認証情報は破棄（最古のものを残す）、
--    その後に重複サイト行を削除する。base_url 完全一致で判定する（既存データは文字列一致）。
CREATE TEMPORARY TABLE sites_dedup ON COMMIT DROP AS
SELECT id,
       first_value(id) OVER (PARTITION BY base_url ORDER BY created_at, id) AS keep_id
FROM sites;

-- 付け替え前に、重複（非keep）サイトのアクティブスキャン（queued/running）を failed にする。
-- keep サイトへ複数のアクティブスキャンが集まると scans_one_active_per_site（1サイト1アクティブ）に
-- 違反するため。統合により放棄されるスキャンなので終端状態に落とす（履歴行は残す）。
UPDATE scans s
SET status = 'failed', finished_at = COALESCE(s.finished_at, now())
FROM sites_dedup d
WHERE s.site_id = d.id AND d.id <> d.keep_id
  AND s.status IN ('queued', 'running');

UPDATE scans s
SET site_id = d.keep_id
FROM sites_dedup d
WHERE s.site_id = d.id AND d.id <> d.keep_id;

DELETE FROM scan_credentials sc
USING sites_dedup d
WHERE sc.site_id = d.id AND d.id <> d.keep_id;

DELETE FROM sites s
USING sites_dedup d
WHERE s.id = d.id AND d.id <> d.keep_id;

-- 3. 残った行の origin を backfill する（target.CanonicalOrigin と同じ正規化）。
UPDATE sites SET origin = lower(regexp_replace(base_url, '^\s*https?://', ''));
UPDATE sites SET origin = regexp_replace(origin, '/.*$', '');
UPDATE sites
SET origin = origin || ':' || CASE WHEN base_url ~* '^https://' THEN '443' ELSE '80' END
WHERE position(':' in origin) = 0;
UPDATE sites SET origin = regexp_replace(origin, '^(127\.0\.0\.1|\[::1\]):', 'localhost:');

-- 4. NOT NULL + UNIQUE を確定する。
ALTER TABLE sites ALTER COLUMN origin SET NOT NULL;
ALTER TABLE sites ADD CONSTRAINT sites_origin_unique UNIQUE (origin);
