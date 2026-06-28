# Frontend Rules — Nuxt 3 + TypeScript

## スタック
Nuxt 3 / TypeScript strict / Tailwind CSS v4 / Vitest / ESLint

## 必須規約

- `<script setup>` + Composition API 必須（Options API 禁止）
- Auto-import を最大限活用
- Tailwind クラスは `web/assets/css/tokens.css` の CSS カスタムプロパティ経由で統一
  - 任意値直指定（`text-[#000000]` / `bg-[#1a1a1a]` 等）禁止
  - 生の hex 値を Vue テンプレート・`<style>` に直書き禁止
- `<style>` ブロックへの CSS 直接記述禁止。`style="..."` インライン属性禁止
  - Tailwind では表現不可能な場合のみ例外。コメントで理由を必ず明記
- 状態管理：`useState` / Pinia / Nuxt composables 優先
- 禁止：`any` 型、`console.log`（本番）、直接 DOM 操作、未使用 import

## コーディング原則

DRY / KISS / YAGNI / 単一責務。

## デザインルール遵守（最優先）

UIを生成・変更する際は必ず以下を参照すること。

| 参照先 | 内容 |
|---|---|
| `DESIGN.md` | カラー・タイポグラフィ・コンポーネント仕様・DO/DONTルール |
| `web/assets/css/tokens.css` | CSS カスタムプロパティ定義（値の正） |

> トークンに未定義の値・色・サイズが必要な場合は、実装前に確認する。勝手に値を追加しない。

## デザイントークン規約

### 禁止事項
- 生の hex 値（`#000000` 等）を Vue テンプレート・`<style>` に直書きしない。必ず `var(--color-*)` 等の CSS 変数を使う
- CSS カスタムプロパティの値を変更する場合は `web/assets/css/tokens.css` のみを編集する

### MTricolor グラデーション
`--gradient-m-stripe` はブランド区切り線専用。CTA・背景塗りへの使用禁止。

### フォント
BMWTypeNextLatin 利用不可時の代替は `Inter`（variable, 700/300 のみ）。
CSS変数: `--font-display: 'BMWTypeNextLatin', 'Inter', sans-serif`

### セキュリティスコアの色分け

| スコア範囲 | 使用トークン |
|---|---|
| 良好（80–100） | `--color-success` |
| 要注意（60–79） | `--color-warning` |
| 危険（40–59） | `--color-m-red` |
| 危機（0–39） | `--color-m-red` + opacity 強調 |

## デザイン変更時の同期（必須）

デザイン・スタイルに関わる変更は、実装と**同時に**以下を更新する。後回し禁止。

| 変更内容 | 更新するファイル |
|---|---|
| 色・余白・タイポグラフィのトークン変更 | `web/assets/css/tokens.css` |
| コンポーネント固有のルール追加 | `.claude/rules/components/<component>.md`（新規作成） |

## スタイル規約（厳守）

### 禁止事項
- `<style>` ブロック（scoped / global 問わず）への CSS 直接記述
- `style="..."` インラインスタイル属性
- 例外：Tailwind では表現不可能な場合のみ `<style>` 使用可。コメントで理由を必ず明記

### Tailwind クラスの書き方
- 色・サイズ・余白はすべて `web/assets/css/tokens.css` の CSS 変数を参照するクラス経由
- `var(--color-canvas)` 等の CSS 変数は `@theme` または直接 `var()` で参照する
- セマンティック命名を使う（`canvas` / `surface-card` / `on-dark` 等。`black` 等の直接参照禁止）

## レスポンシブ実装

- **Mobile-first** で実装する（モバイルをベースに `md:` `lg:` で上書き）
- ブレークポイント（DESIGN.md 準拠）:
  - `< 768px` — Mobile
  - `768–1024px` — Tablet
  - `> 1024px` — Desktop
- 固定 px 幅（`w-[375px]` 等）禁止。`w-full` / `max-w-*` / `grid-cols-*` で可変にする

## コンポーネント設計

- Atomic Design 風に整理（atoms / molecules / organisms）
- 1コンポーネント1責務。Props は型安全に定義
- ダッシュボードの Chart.js グラフは `components/dashboard/` に集約する

## セキュリティ固有の UI ルール

- 認証情報（Cookie / Bearer）を入力するフォームは **マスク表示** でプレビューする
- スキャン結果・ findings は重大度カラー（`--color-success` / `--color-warning` / `--color-m-red`）で表示する
- スコアカラーは本ファイル内「セキュリティスコアの色分け」を参照する
- ドメイン所有確認ガイドは初心者向けに手順を図解する（テキストだけにしない）

## API 連携

- エラーハンドリングは composable（`useApiError` 等）に集約する
- スキャン進捗（queued → running → done）はポーリングまたは SSE で取得する

## テスト方針

- Vitest + @nuxt/test-utils
- 新機能時は期待出力・テストケースを先に明記（TDD推奨）

### カバレッジ要件

Vitest（Istanbul）が報告する以下の指標を全て満たすこと。

| 指標 | 目標 |
|---|---|
| Statements（≈ C0） | **100%** |
| Branches（≈ C1） | **100%** |
| Functions | **100%** |

**除外対象**（`vitest.config.ts` の `coverage.exclude` に指定）:
- `*.d.ts` / 型定義ファイル
- `nuxt.config.ts` / `tailwind.config.ts` 等の設定ファイル
- `assets/` / `public/` 配下の静的ファイル

**CI での確認コマンド**:
```bash
pnpm test --coverage
```
