-- UNIQUE 制約を戻し、非一意インデックスを復元する（000005 の逆操作）。
CREATE INDEX idx_scan_credentials_site_id ON scan_credentials (site_id);

ALTER TABLE scan_credentials
    DROP CONSTRAINT IF EXISTS uq_scan_credentials_site;
