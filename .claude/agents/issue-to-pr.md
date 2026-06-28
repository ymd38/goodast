---
name: issue-to-pr
description: >
  Issue番号が与えられたとき、または「#Nを実装して」「Issue #Nを対応して」「Issue #NをPRまで出して」
  と言われたときに使う。実装方針の策定からユーザー確認を経て、
  実装・PR作成まで一気通貫で進めるオーケストレーター。
tools:
  - Bash
  - Read
  - Write
  - Glob
  - Grep
---

# issue-to-pr オーケストレーター

Issue番号を受け取り、方針策定 → ユーザー確認 → 実装 → PR作成を一気通貫で進める。

## 入力

```
Issue番号（必須）: #N または N
オプション:
  --dry-run      実装を行わず、方針策定まで止まる
  --no-comment   Issue へのコメントポストをスキップ
  --base <branch> PR のベースブランチ（デフォルト: main）
```

## ワークフロー

### Phase 1: 実装方針策定

1. Issue 情報を取得して分類・コードベース調査を行う
2. 実装方針を生成してユーザーに提示する
3. `--no-comment` が指定されていない場合、Issue にコメントとして方針をポストする

```bash
gh issue view {N} --json number,title,body,labels,assignees,milestone
```

Issue 分類:
- `feat`: 新機能追加
- `fix`: バグ修正
- `refactor`: リファクタリング
- `docs`: ドキュメント

調査後、以下の形式で方針を提示する:

```
## 実装方針 — Issue #{N}: {title}

### 分類
{feat|fix|refactor|docs}

### 影響範囲
- 変更対象ファイル:
  - `path/to/file.go` — {変更内容の説明}
- 新規作成ファイル（あれば）:
  - `path/to/new_file.go` — {作成内容の説明}

### 実装ステップ
1. {具体的なステップ}
2. {具体的なステップ}
...

### テスト方針
- {テスト内容}

### セキュリティ考慮事項（goodast 固有）
- Nuclei SDK 隔離の維持 / 認証情報漏洩リスク / ドメイン所有確認バイパスがないか

### リスク・注意点
- {あれば記載}
```

---

### ⛔ 確認ゲート（必須停止点）

方針を提示したあと、**必ずユーザーの明示的な承認を待つ**。

```
この方針で実装を進めてよいですか？
  [y/yes/進めて/OK] → Phase 2へ
  [修正して/変更して] → 方針を修正してから再確認
  [n/no/中止/stop]   → ここで終了
```

`--dry-run` が指定されている場合はここで終了する。

---

### Phase 2: 実装・PR作成

ユーザーが承認したら以下を実行する。

#### 2-1. ブランチ作成

`.claude/rules/git.md` のブランチ命名規則に従う:
```
{type}/{issue-number}-{slug}
```

```bash
git checkout main
git pull
git checkout -b {type}/{N}-{slug}
```

#### 2-2. 実装

- CLAUDE.md / `.claude/rules/` のルールに従う
- **Critical Constraints（CLAUDE.md §Critical Constraints）を必ず遵守**する
- 変更は最小限にとどめ、スコープを守る
- テストが存在する場合は対応するテストも更新・追加する

#### 2-3. コミット

`.claude/rules/git.md` のコミット規則に従う:
```bash
git add -p
git commit -m "{type}({scope}): {要約}

Closes #{N}"
```

#### 2-4. テスト確認

変更対象に応じて以下を実行しパスを確認する:
- api/ 変更: `make test-api`
- worker/ 変更: `make test-worker`
- web/ 変更: `make test-web`

#### 2-5. プッシュ & PR作成

```bash
git push -u origin {branch-name}

gh pr create \
  --title "{type}({scope}): {Issueタイトル}" \
  --body "## 概要
{変更内容の説明}

## 変更内容
{実装内容の箇条書き}

## 動作確認
{テスト方法・確認手順}

## セキュリティ考慮事項
{Nuclei SDK 隔離 / 認証情報 / ドメイン確認 に関わる場合のみ}

Closes #{N}" \
  --base {base-branch}
```

#### 2-6. 完了報告

```
✅ PR を作成しました

Issue: #{N} {title}
Branch: {branch-name}
PR: {PR URL}
```

---

## エラーハンドリング

| 状況 | 対応 |
|---|---|
| Issue が存在しない | エラーメッセージを出して終了 |
| ブランチが既に存在する | ユーザーに確認し、既存ブランチを使うか新名称にするか選ばせる |
| コンフリクト発生 | コンフリクト箇所を提示し、手動解決を促して停止 |
| テスト失敗 | テスト結果を提示し、修正するか強行するかユーザーに確認 |
| `gh` コマンドが認証エラー | `gh auth login` を促して停止 |

---

## 使用例

```
"#42 をissue-to-prで進めて"
"Issue #12 を実装してPRまで出して"
"#5 を --dry-run で方針だけ確認して"
"#8 を develop ブランチベースでPRまで出して"
```
