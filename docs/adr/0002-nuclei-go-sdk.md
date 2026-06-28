# ADR-0002: エンジンにNuclei v3 Go SDKを採用しバージョン固定する

Status: Accepted
Date: 2026-06-27

## Context

goodast は脆弱性検査エンジンをゼロから作らず、実績あるOSSをラップして「UI × 可視化」に開発リソースを集中する方針。候補はNucleiとZAP。

- Nuclei v3 は公式Go SDK（`github.com/projectdiscovery/nuclei/v3/lib`、MIT）を提供。`NewNucleiEngine()` で生成し `ExecuteWithCallback()` で結果をコールバック取得できる。`ThreadSafeNucleiEngine` で並列実行も可能。
- goodast のバックエンドはGoなので、Nucleiは**ネイティブに埋め込め、別言語サイドカーが不要**。
- ZAPはJava製でGo埋め込み不可。Dockerサイドカー＋REST APIでの連携になる。
- Nucleiは `-dast`（ファジング）テンプレートでインジェクション・XSS・SSRF等をカバーし、テンプレート方式で誤検知が少ない。

## Decision

PoCのエンジンに **Nuclei v3 Go SDK** を採用する。ZAPはフェーズ2で必要になった時点（クロール・アクティブスキャン・フォーム認証強化）にDockerサイドカーとして追加する。

Nucleiのバージョンは**コンフィグで明示固定**する。テンプレートも特定バージョンを起動時に取得し、リポジトリには同梱しない（容量・ライセンス管理の観点）。

## Consequences

- **利点**: Goネイティブ統合で構成がシンプル。MITライセンスでOSS化に支障なし。テンプレートコミュニティの成果を即利用できる。
- **トレードオフ**: SDKは破壊的変更がありうるため、バージョン固定とワーカー隔離（ADR-0001）で影響を抑える。テンプレート更新で挙動が変わらないよう、固定バージョンを明示的に上げる運用とする。
- Nuclei自体のテンプレート由来リスク（過去にローカルファイル読取・RCEの修正実績あり）に備え、信頼できるテンプレートソースのみ使用する。
