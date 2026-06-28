@../CLAUDE.md
@../.claude/rules/backend.md

# API — 補足事項

## 責務
スキャン受付・サイト管理・ドメイン所有確認・認証情報の暗号化保管・ダッシュボード用データ提供。
**Nuclei SDK は絶対にここへ import しない（ADR-0002）。**

## コマンド（api/ 内）

```bash
go run ./cmd/api/
go test ./... -race -v
golangci-lint run --fix
go mod tidy
```

変更後は必ず `go test ./... -race` と lint を実行してパスを確認。
