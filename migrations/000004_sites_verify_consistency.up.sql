-- sites.verify_method と verify_token は「両方 NULL（ローカル対象）」または
-- 「両方 NOT NULL（所有確認対象）」のいずれかに限定する。片方だけ設定された不整合行は
-- レスポンス整形（設置ガイド生成）や確認フローを壊すため、DB レベルで根本的に防ぐ。
ALTER TABLE sites
    ADD CONSTRAINT sites_verify_method_token_consistency
    CHECK ((verify_method IS NULL) = (verify_token IS NULL));
