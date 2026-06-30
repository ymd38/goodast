# Backend Rules — Go + Gin + river + pgx

## スタック
Go 1.26.4 / Gin / sqlc + pgx v5 / golang-migrate / river（ジョブキュー）/ testify / slog（構造化ログ）/ go.uber.org/dig（DI）

> **生SQLを直接Goコードに書かない。** `queries/*.sql` に SQLを書き、sqlcで型安全なGoコードを生成する。

## フォルダ構造

```
api/
  cmd/api/         エントリーポイント（dig コンテナ構築はここのみ）
  internal/
    config/        環境変数の読み込み・バリデーション
    db/
      queries/     *.sql ファイル（sqlc の入力）
      *.go         sqlc 生成コード（編集禁止）
    handler/       Gin ハンドラ（HTTPレイヤー。service にのみ依存）
    site/          サイト登録・ドメイン所有確認（feature パッケージ）
      service.go     ビジネスロジック（usecase 相当・gin 非依存）
      repository.go  sqlc Queries のラッパー（infrastructure 相当）
      verify.go      ドメインロジック（所有確認の純粋関数）
      types.go       ドメイン型（sqlc struct と乖離する場合のみ）
    scan/          スキャンジョブ受付・enqueue（同上の内部構成）
    report/        スコア計算・ダッシュボードデータ集計
      score.go       スコア計算（純粋関数・テスト最重要）
      dashboard.go   ダッシュボード集計
  sqlc.yaml        sqlc 設定ファイル

worker/
  cmd/worker/      エントリーポイント（dig コンテナ構築はここのみ）
  internal/
    config/        環境変数の読み込み・バリデーション
    db/
      queries/     *.sql ファイル（sqlc の入力）
      *.go         sqlc 生成コード（編集禁止）
    engine/        スキャンエンジン（Nuclei SDK ラッパーはここのみ）
      engine.go      Engine インターフェース（将来 ZAP 追加を見据える）
      nuclei.go      Nuclei 実装・ガードレール適用
  sqlc.yaml        sqlc 設定ファイル
```

## sqlc 運用規約

- SQL は `api/internal/db/queries/*.sql` に書く。Goコードへの生SQL直書き禁止
- `sqlc generate` で生成したコードは編集しない（再生成で上書きされる）
- 動的条件（`WHERE` 句の任意組み合わせ等）は `sqlc` の `CASE` / `COALESCE` パターンか、クエリを複数に分けて対応する
- `pgx.Conn` / `pgx.Pool` を直接操作するのは sqlc 生成コードの内部のみ
- マイグレーション変更後は必ず `sqlc generate` を再実行する

## 必須規約

- **エラー**: `fmt.Errorf("....: %w", err)` でラップ。`errors.Is` / `errors.As` で判定
- **context.Context** を全レイヤーで伝播させる（省略しない）
- **DBマイグレーション**: golang-migrate のみ。AutoMigrate 相当は禁止
- **バリデーション**: Ginのバインディング + ドメイン層の両方で実施
- **panic**: 最小限。recoveryはGinミドルウェアに任せる

## コーディング原則

DRY / KISS / YAGNI。インターフェースは「2実装以上になるとき」だけ切る。

## アーキテクチャ方針（package-by-feature）

フルのクリーンアーキテクチャ（domain/usecases/services/infrastructure を最上位レイヤーで分割）は**採用しない**。
sqlc・YAGNI・実DBテスト方針と衝突し、1機能の修正で複数ディレクトリを横断するコストに見合わないため。
代わりに **package-by-feature**（`site/` `scan/` `report/` 等を bounded context として切る）を採用し、各 feature の**内部**を責務でファイル分割する。

### feature 内のファイル責務

| ファイル | 責務 | 制約 |
|---|---|---|
| `service.go` | ビジネスロジック（usecase 相当） | `gin` / `net/http` を import しない |
| `repository.go` | sqlc `Queries` のラッパー（永続化） | SQL直書き禁止（sqlc経由のみ） |
| `<domain>.go`（例: `verify.go` / `score.go`） | ドメインロジック（純粋関数優先） | 副作用を持たせない・テスト容易に保つ |
| `types.go` | ドメイン型 | sqlc struct と乖離する場合のみ作る（不要なら作らない） |

### 依存方向のルール（2つだけ）

1. **handler は service にのみ依存する。** HTTP関心事（バインド・ステータスコード・レスポンス整形）だけを書き、ビジネスロジックを持たない
2. **service は `gin` / `net/http` を import しない。** 純粋に保ち、HTTPなしでユニットテストできる状態を維持する

### ドメイン型と sqlc struct

- sqlc 生成struct は**永続化境界のデータキャリア**。不変条件・振る舞いを持たない単純データはそのまま使ってよい（意味のない型ラップはしない＝YAGNI）
- 不変条件・振る舞いを持つ概念はドメイン型に包む（→「ドメイン型とカプセル化」）。**repository が sqlc row ↔ ドメイン型を変換する唯一の境界**となる（この一箇所のマッピングは許容）
- service が DB に依存する箇所は、まず `*db.Queries` 具象に依存してよい（実DBテストで検証）。モック化が必要になった時点で sqlc 生成の `db.Querier` インターフェースに差し替える

### インターフェースを切る箇所（例外）

- `worker/internal/engine/` の `Engine` インターフェース — フェーズ2で ZAP を追加予定（Nuclei / ZAP の2実装）。「2実装以上」ルールに合致する唯一の正当化箇所

## ドメイン型とカプセル化（プリミティブ依存の排除）

意味を持つデータを生のプリミティブ（`string` / `int` / `uuid.UUID` / `[]byte` 等）のまま層をまたいで渡さない。
データと、そのデータを操作するロジックを**同じ型に同居**させ、同じ変換・判定が複数箇所に散らばるのを防ぐ（Tell, Don't Ask）。

### 原則

- **値オブジェクト化**: 不変条件を持つ概念はコンストラクタ `NewXxx(...) (Xxx, error)` で生成時に検証し、**不正な値のインスタンスを作れない**状態にする
- **振る舞いの同居**: その値に対する操作（変換・判定・整形）は型のメソッドに集約する。service / handler 側に同じロジックを再実装しない
- **識別子の型付け**: ID は裸の `uuid.UUID` / `string` でなく `type SiteID …` のように型付けし、引数の取り違え（`ScanID` を `SiteID` 引数に渡す等）を**コンパイル時に**防ぐ
- **列挙の型付け**: 状態・区分は裸の string 定数でなく named type + 定義済み値に限定する（不正値をパースで弾く）

### goodast での具体例

| 概念 | プリミティブ（NG） | ドメイン型（推奨） | 型に同居させる操作 |
|---|---|---|---|
| スコア | `int` | `Score` | ラベル・色の決定、前回との差分計算 |
| 重大度 | `string` | `Severity` | 重み・比較・パース（不正値拒否） |
| スキャン対象URL | `string` | `TargetURL` | スコープ allowlist 判定・危険パス判定 |
| 所有確認トークン | `string` | `VerifyToken` | 生成・照合 |
| 暗号化ヘッダ | `[]byte` | `EncryptedHeaders` | 暗号化／復号、`String()` で必ずマスク（平文ログ防止・ADR-0003） |

```go
// 値オブジェクトの例: 不正な Score を作れない + 操作を同居させる
type Score struct{ v int }

func NewScore(v int) (Score, error) {
    if v < 0 || v > 100 {
        return Score{}, fmt.Errorf("score out of range: %d", v)
    }
    return Score{v: v}, nil
}

func (s Score) Value() int       { return s.v }
func (s Score) Label() string    { /* 80+ 良好 ... の判定をここに集約 */ }
func (s Score) Delta(prev Score) int { return s.v - prev.v }
```

> sqlc struct との境界は repository で吸収する（上記「ドメイン型と sqlc struct」参照）。
> 不変条件も振る舞いも持たない単純データまで機械的に包む必要はない（YAGNI）。「意味と不変条件があるか」で判断する。

## 本番運用規約（Day 1 から実装する）

### 構造化ログ（slog）
- `log/slog` を全レイヤーで使用。`fmt.Println` / `log.Printf` 禁止
- レベル: `Debug`（開発時詳細）/ `Info`（通常操作）/ `Warn`（要注意）/ `Error`（要調査）
- リクエストID・サイトID・スキャンIDを必ずフィールドに含める
- 平文ログ禁止は認証情報だけでなく全フィールドに適用（ADR-0003）

### Graceful Shutdown
- API・Worker ともに `os.Signal`（`SIGTERM` / `SIGINT`）を受け取りシャットダウンする
- API: `http.Server.Shutdown(ctx)` で進行中リクエストを完了させてから終了
- Worker: 実行中スキャンジョブを完了（またはキャンセル）してから終了。river の `gracefulShutdown` を使う
- シャットダウンタイムアウト: 30秒（コンフィグで変更可）

### ヘルスチェックエンドポイント
- `GET /healthz` — プロセス死活確認（DB接続不問。Kubernetes liveness probe 用）
- `GET /readyz` — DB接続・依存サービス疎通確認（readiness probe 用）
- Worker にも同様の HTTP ヘルスポートを立てる（デフォルト: `:9090`）

### 設定管理（12-factor）
- 設定は**すべて環境変数**から読む。コードへのハードコード禁止
- `config` パッケージを `internal/config/` に置き、起動時に一括バリデーション
- 必須変数が未設定の場合は起動を失敗させる（サイレントなデフォルト値で動き続けない）
- 秘密情報（暗号化キー等）は環境変数のみ。設定ファイルに書かない

### DB接続プール（pgxpool）
- `pgxpool.Config` を明示的に設定する（デフォルト任せ禁止）
- 推奨初期値: `MaxConns: 10`, `MinConns: 2`, `MaxConnLifetime: 30m`, `HealthCheckPeriod: 1m`
- 本番値はロードテスト結果で調整する

### 依存性注入（go.uber.org/dig）

- `dig.Container` の構築は `cmd/*/main.go` **のみ**。`internal/` 配下で container を触らない
- 依存の登録は `container.Provide`、起動処理は `container.Invoke`
- struct-based injection（`dig.In` / `dig.Out`）を使う。引数が3つ以上になる関数はすべて struct に束ねる
- コンストラクタは `NewXxx(deps XxxDeps) (*Xxx, error)` 形式で統一
- グローバル変数による依存解決禁止（dig のコンテナが唯一の配線場所）
- インターフェースは「2実装以上になるとき」だけ切る（YAGNI。dig の `Name` タグで複数実装を区別）

```go
// 例: struct-based injection
type HandlerDeps struct {
    dig.In
    SiteService *site.Service
    ScanService *scan.Service
    Logger      *slog.Logger
}
```

### Dockerfile
- multi-stage build（builder → distroless or scratch）でイメージを最小化
- 実行ユーザーは non-root（UID 1001）
- API と Worker は別イメージとしてビルドする（Nuclei SDKを含むWorkerイメージを分離）

## Nuclei SDK（worker/internal/engine/ のみ）

- Nuclei SDK のインポートは `worker/` ディレクトリ以外で禁止（ADR-0002）
- Nuclei バージョンは `go.mod` で固定し `go get` による自動更新を行わない
- スキャン結果はコールバックで受け取り、goroutine-safe に DB 保存する
- ガードレール（allowlist・危険パス除外・レート制限）は `engine/` レイヤーで適用する

## ジョブキュー（river）

- enqueue: api/internal/scan/ から river クライアントで PostgreSQL にジョブを積む
- dequeue: worker が river で取り出しスキャンを実行する
- 新規ブローカー（Redis等）を追加しない

## 認証情報の暗号化

- Cookie / Bearer はアプリケーションレイヤーで暗号化して `scan_credentials.enc_headers` に保存
- pgcrypto（DB側暗号化）は使わない（ADR-0003）
- ログへの平文出力禁止・API レスポンスでの生値返却禁止（マスク必須）

## DBマイグレーション

- ファイル: `migrations/` 配下に連番で管理
- 開発: `make migrate` で適用
- 本番: デプロイ前に手動または CI で `make migrate` を実行
- AutoMigrate は禁止

## テスト方針

- テーブル駆動テスト（Table Driven Tests）を基本とする
- `-race` フラグ必須（競合検知）
- DB を使うテストは `docker-compose` の test DB またはtestcontainers を使用
- Nuclei SDK を呼ぶテストはモックせず integration test として分離する（`//go:build integration`）

### カバレッジ要件

| 指標 | 目標 | 計測方法 |
|---|---|---|
| C0（命令網羅） | **100%** | `go test -covermode=atomic ./...` |
| C1（分岐網羅） | **100%** | テーブル駆動テストで true/false 両パスを網羅することで担保 |
| C2（条件網羅） | **80%** | PR レビューで複合条件の組み合わせをチェック |

**除外対象**（カバレッジ計測から外す）:
- `internal/db/*.go` — sqlc 生成コード（編集禁止・再生成で消える）
- `cmd/*/main.go` — エントリーポイント（DI の配線のみ）
- `//go:build integration` のファイル — 別途 integration テストとして計測
- `worker/internal/engine/nuclei` — Nuclei SDK アダプタ。SDK 呼び出しはネットワーク＋テンプレートを
  要しユニットテスト不可のため、//go:build integration で検証する（ADR-0002）。純粋ロジック
  （スコープ判定・severity 正規化・集計）は親パッケージ `engine` に置き 100% ユニット網羅する

> 補足: `worker/internal/scanjob` は river/DB と結合する orchestration のため integration テストで
> 網羅する（unit テストを持たない）。純粋ロジックは `engine` に切り出して unit 計測する方針。

**CI での確認コマンド例**:
```bash
go test -race -covermode=atomic -coverprofile=coverage.out \
  $(go list ./... | grep -v '/db$\|/cmd/\|/engine/nuclei$\|/scanjob$')
go tool cover -func=coverage.out | grep -E "^total" 
```

## 禁止事項

- グローバル変数の多用
- ハードコードされた秘密情報・APIキー
- 未使用 import
- **Goコードへの生SQL直書き**（`fmt.Sprintf("SELECT ... WHERE id = %s", id)` 等）
- `worker/internal/engine/` 以外での Nuclei SDK import
