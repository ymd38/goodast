@../CLAUDE.md
@../.claude/rules/backend.md

# Worker — 補足事項

## 責務
river でジョブをデキューし、Nuclei SDK を使って非同期スキャンを実行。結果をコールバックで受けてDBへ保存。

## ⚠ 最重要制約
- Nuclei SDK は **`worker/internal/engine/` にのみ**置く
- api/ や他のパッケージから `worker/internal/engine/` を import しない
- Nuclei バージョンは `go.mod` で固定。`go get -u` を実行しない（ADR-0002）

## コマンド（worker/ 内）

```bash
go run ./cmd/worker/
go test ./... -race -v
go test -tags=integration ./... -race -v   # Nuclei を実際に呼ぶ統合テスト
golangci-lint run --fix
go mod tidy
```

変更後は必ず `go test ./... -race` と lint を実行してパスを確認。

## ガードレール実装場所

engine/ レイヤーで以下を強制する:
- allowlist チェック（登録ドメイン外へのリクエスト禁止）
- 危険パス除外（logout / signout / delete / remove / destroy / admin/* 等）
- レート制限（デフォルト低 req/s）
- 破壊的テンプレートのデフォルト無効
