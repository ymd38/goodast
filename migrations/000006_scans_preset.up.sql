-- scans に実行プリセット（軽量/標準/詳細）を記録する。履歴・ダッシュボード表示の記録用。
-- 既存行は安全側の standard で埋める。CHECK は jobs.Preset の値集合と一致させる。
ALTER TABLE scans
    ADD COLUMN preset text NOT NULL DEFAULT 'standard'
    CHECK (preset IN ('light', 'standard', 'deep'));
