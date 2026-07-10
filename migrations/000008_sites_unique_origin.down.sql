-- origin 一意制約と列を撤去する。
-- 注: 重複行の一元化（scans の付け替え・重複サイト削除）は不可逆のため戻さない。
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_origin_unique;
ALTER TABLE sites DROP COLUMN IF EXISTS origin;
