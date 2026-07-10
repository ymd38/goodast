-- サイトを「ドメイン+ポート（origin）」で一意にする。同一 origin の重複登録を防ぎ、
-- サイト管理・スキャン履歴を1サイトに一元化する。
--
-- origin の正規化ルールは api の target.CanonicalOrigin と一致させる:
--   - スキーム除去・userinfo 除去・パス/クエリ/フラグメント除去して host[:port] を得る
--   - ポート省略時はスキーム既定（https=443 / それ以外=80）を補う
--   - ホストは小文字化し、ループバック別名（127.0.0.1 / ::1）は localhost に畳み込む
-- ※アプリ層（target.CanonicalOrigin）が今後の唯一の正。本 backfill は既存行を
--   一意化するための best-effort。dedup は base_url 文字列ではなく正規化後の origin で行う
--   （異なる URL 表記でも同一 origin なら 1 行に集約する）。

-- 1. origin 列を追加（まずは nullable で backfill する）。
ALTER TABLE sites ADD COLUMN origin text;

-- 2. dedup より先に origin を backfill する（正規化後の値で重複判定するため）。
--    target.CanonicalOrigin と同じ順序で段階的に正規化する。
UPDATE sites SET origin = lower(base_url);
-- スキーム除去（先頭空白も一緒に）。
UPDATE sites SET origin = regexp_replace(origin, '^\s*https?://', '');
-- userinfo（user:pass@）除去。@ より前が authority 内にある場合のみ。
UPDATE sites SET origin = regexp_replace(origin, '^[^/?#]*@', '');
-- パス/クエリ/フラグメント除去（authority 部だけ残す）。
UPDATE sites SET origin = regexp_replace(origin, '[/?#].*$', '');
-- ポート未指定ならスキーム既定を補う。IPv6（[..]）は末尾が ']' で終わるため
-- 「:数字で終わらない」判定で明示ポートの有無を正しく見分けられる。
UPDATE sites
SET origin = origin || ':' || CASE WHEN base_url ~* '^\s*https://' THEN '443' ELSE '80' END
WHERE origin !~ ':[0-9]+$';
-- ループバック別名を localhost に畳み込む（127.0.0.1 / [::1]）。
UPDATE sites SET origin = regexp_replace(origin, '^(127\.0\.0\.1|\[::1\]):', 'localhost:');

-- 3. 同一 origin を最古行へ一元化する。
--    スキャンは最古サイトへ付け替え、重複サイトの認証情報は破棄（最古のものを残す）、
--    その後に重複サイト行を削除する。
CREATE TEMPORARY TABLE sites_dedup ON COMMIT DROP AS
SELECT id,
       first_value(id) OVER (PARTITION BY origin ORDER BY created_at, id) AS keep_id
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

-- 4. NOT NULL + UNIQUE を確定する。
ALTER TABLE sites ALTER COLUMN origin SET NOT NULL;
ALTER TABLE sites ADD CONSTRAINT sites_origin_unique UNIQUE (origin);
