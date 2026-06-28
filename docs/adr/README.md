# Architecture Decision Records (ADR)

このディレクトリは goodast の設計上の意思決定を1決定1ファイルで記録する。
「なぜそうなっているか」をClaude Code・将来の自分が参照できるようにし、手戻りを防ぐ。

| ADR | タイトル | ステータス |
|---|---|---|
| [0001](0001-api-worker-separation.md) | APIとスキャンワーカーを分離する | Accepted |
| [0002](0002-nuclei-go-sdk.md) | エンジンにNuclei v3 Go SDKを採用しバージョン固定する | Accepted |
| [0003](0003-app-layer-encryption.md) | 認証情報はアプリケーションレイヤーで暗号化する | Accepted |
| [0004](0004-domain-ownership-verification.md) | スキャン前にドメイン所有確認を必須とする | Accepted |
| [0005](0005-river-job-queue.md) | ジョブキューにriverを採用する | Accepted |

## フォーマット

各ADRは Context（背景）/ Decision（決定）/ Consequences（結果・トレードオフ）の3節で書く。
