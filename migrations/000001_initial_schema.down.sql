-- 依存の逆順で削除する。インデックスはテーブル削除に伴って消える。
DROP TABLE IF EXISTS findings;
DROP TABLE IF EXISTS scans;
DROP TABLE IF EXISTS scan_credentials;
DROP TABLE IF EXISTS sites;
