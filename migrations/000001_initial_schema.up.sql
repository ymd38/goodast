-- 初期スキーマ（企画書 §5）。UUID は PG16 組込の gen_random_uuid() を使う（拡張不要）。
-- 列挙値は text + CHECK 制約で表現する（enum の硬直性を避け、Go 側はドメイン型で型安全を担保）。

-- sites: 診断対象サイト。name を診断履歴のキーにする。
CREATE TABLE sites (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text NOT NULL UNIQUE,
    base_url           text NOT NULL,
    ownership_verified boolean NOT NULL DEFAULT false,
    verify_method      text CHECK (verify_method IN ('dns-txt', 'file')),
    verify_token       text,
    created_at         timestamptz NOT NULL DEFAULT now()
);

-- scan_credentials: セッション持ち込み認証情報。enc_headers はアプリ層で暗号化（ADR-0003）。
CREATE TABLE scan_credentials (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id     uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    auth_mode   text NOT NULL DEFAULT 'none' CHECK (auth_mode IN ('none', 'session')),
    enc_headers bytea,
    created_at  timestamptz NOT NULL DEFAULT now(),
    -- session モードでは暗号化ヘッダ必須、none では NULL に強制する（ADR-0003 / 整合性）。
    CONSTRAINT scan_credentials_session_headers CHECK (
        (auth_mode = 'none' AND enc_headers IS NULL)
        OR (auth_mode = 'session' AND enc_headers IS NOT NULL)
    )
);

-- scans: 1回のスキャン = 時系列の1点。
CREATE TABLE scans (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id        uuid NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    status         text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'done', 'failed')),
    engine_version text,
    started_at     timestamptz,
    finished_at    timestamptz,
    summary_json   jsonb,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- findings: 個別の検出結果。DAST のため検出箇所は file:line でなく URL。
CREATE TABLE findings (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id     uuid NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    template_id text NOT NULL,
    title       text NOT NULL,
    severity    text NOT NULL
        CHECK (severity IN ('Critical', 'High', 'Medium', 'Low', 'Info')),
    url         text NOT NULL,
    cwe         text,
    remediation text,
    status      text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'false_positive', 'fixed')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- インデックス: FK 検索とダッシュボードの時系列描画用。
CREATE INDEX idx_scan_credentials_site_id ON scan_credentials (site_id);
CREATE INDEX idx_scans_site_id_created_at ON scans (site_id, created_at DESC);
CREATE INDEX idx_findings_scan_id ON findings (scan_id);
