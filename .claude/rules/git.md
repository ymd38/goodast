# Git 運用規約

## ブランチ命名規則

```
<type>/<issue-number>-<kebab-case-summary>
```

### type 一覧
| type | 用途 |
|---|---|
| `feat` | 新機能追加 |
| `fix` | バグ修正 |
| `refactor` | 機能変更なしのリファクタリング |
| `style` | UI・スタイルのみの変更（ロジック変更なし）|
| `docs` | ドキュメント・コメントのみ |
| `test` | テストの追加・修正のみ |
| `chore` | ビルド・設定・依存関係の変更 |
| `db` | マイグレーションファイルの追加・修正 |

### 例
```
feat/12-site-registration
fix/23-nuclei-callback-panic
style/8-dashboard-score-color
db/5-add-findings-table
```

## コミットメッセージ規約（Conventional Commits）

```
<type>(<scope>): <summary>

[body（任意）]
```

### ルール
- summary は命令形・現在形で書く（日本語可）。過去形・体言止め禁止
- summary は72文字以内
- scope はオプション。影響範囲が明確な場合に使う
- body は変更の「なぜ」を書く。「何を変えたか」はコードを見ればわかる

### goodast の主要 scope
`site` / `scan` / `worker` / `engine` / `report` / `auth` / `dashboard` / `db` / `infra`

### 例
```
feat(site): ドメイン所有確認のファイル設置方式を実装する
fix(engine): Nuclei コールバックで panic が発生するケースを修正する
refactor(report): スコア計算ロジックを internal/report に集約する
style(dashboard): スコアカラーを design token の CSS 変数に統一する
db: findings テーブルに status カラムを追加するマイグレーションを作成する
chore: docker-compose に juiceshop プロファイルを追加する
```

### 禁止パターン
```
# NG：なぜ が不明
fix: バグ修正

# NG：過去形
feat: 認証を追加した

# NG：issue 番号だけ
#42

# NG：WIPをそのままPRに含める
WIP: 途中
```

## PR（Pull Request）規約

### タイトル
コミットメッセージと同じ形式で書く。
```
feat(site): ドメイン所有確認フローを実装する
```

### 本文テンプレート
```markdown
## 概要
<!-- 何を・なぜ変更したか、1〜3行で -->

## 変更内容
<!-- 主な変更点を箇条書き -->

## 動作確認
<!-- 確認した手順・環境 -->

## 関連 Issue
closes #<issue-number>
```

### PR のルール
- 1PR = 1つの目的。複数の機能を混在させない
- バックエンド変更は `make test-api` または `make test-worker` がパスしていること
- フロント変更は `make test-web` がパスしていること
- マイグレーションを含む場合は `make migrate` 実行確認を PR 本文に記載する
- レビュー前にセルフレビュー（diff を一読）すること
- **Nuclei SDK の変更を含む場合**（worker/internal/engine/）はセキュリティレビューを必ず記載する

## その他
- `main` ブランチへの直接 push 禁止
- マージ方式：Squash merge を基本とする
- ブランチは PR マージ後に削除する
