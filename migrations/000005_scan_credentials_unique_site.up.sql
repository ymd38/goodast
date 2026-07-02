-- scan_credentials は site ごとに1組（session 認証情報）とする（ADR-0003）。
-- 「none」は行の不在で表現し、session 設定時のみ行を作る（upsert）。そのため site_id に
-- UNIQUE 制約を張り、ON CONFLICT (site_id) による upsert を可能にする。
ALTER TABLE scan_credentials
    ADD CONSTRAINT uq_scan_credentials_site UNIQUE (site_id);

-- UNIQUE 制約が site_id の一意インデックスを作るため、既存の非一意インデックスは冗長。削除する。
DROP INDEX IF EXISTS idx_scan_credentials_site_id;
