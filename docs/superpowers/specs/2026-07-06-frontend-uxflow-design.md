# UI一気通貫フロー（v1・コア4画面）— 設計

最終更新: 2026-07-06

PoC のコンセプト「UI起点で診断の状態と遷移を可視化」を、ブラウザだけで
**サイト登録 → 所有確認 → スキャン設定 → 実行/進捗 → 結果レポート**まで一気通貫で
体験できる状態にする frontend（web/ Nuxt 3）実装の設計。

> backend API は全て実装済み。本設計は既存の frontend パターン（型付き `useApiClient`、
> `useAsyncData({server:false})`、tokens.css セマンティッククラス、Vitest 100% ゲート）に
> 全面的に沿う。実装は CLAUDE.md の front/back 別セッション規約に従い、**別の frontend
> セッション**で本 spec 由来のプランを実行する。

## スコープ（承認済み）

- **v1 = コア4画面**: ①サイト登録（+所有確認ガイド）→ ②スキャン設定ウィザード（プリセット
  選択）→ ③進捗ポーリング → ④結果レポート。
- **v1 非対象（後続）**: 認証持込（Cookie/Bearer マスク入力・`PUT /sites/:id/credentials`）、
  診断履歴画面（`GET /sites/:id/scans`）。
- **危険パス除外は情報表示のみ**（機能トグルではない。engine 側で常時デフォルト除外・API
  パラメータ無し）。
- 実装は **3 PR に段階化**（A: 登録+所有確認 / B: ウィザード+スキャン開始 / C: 進捗+結果）。
  spec は1本（フロー共通の型・エラー処理・デザインを共有するため）。

## 対象 API（実装済み・契約は swagger.yaml が正）

| 画面 | API | 主なレスポンス |
|---|---|---|
| ①登録 | `POST /sites` | `siteResponse`（`id`/`name`/`base_url`/`ownership_verified`/`verify_method`/`verify_token`/`verification{method,file_path,file_content,dns_record}`/`created_at`）。localhost 等は即 `ownership_verified=true` |
| ①確認 | `POST /sites/:id/verify` | `siteResponse`（200 成功 / **422** 未達） |
| ②開始 | `POST /scans` `{site_id, preset}` | 202 `{scan_id, status, preset}`（不正 preset は 400） |
| ③進捗 | `GET /scans/:id` | `ScanState`（`status`=queued/running/done/failed・`summary{counts,score,band,label}` は done で非 null） |
| ④結果 | `GET /scans/:id/findings` | `Finding[]`（`template_id`/`title`/`severity`/`url`/`cwe`/`remediation`/`status`）。0件は空配列 |

プリセット値は backend の `jobs.Preset`: `light`（軽量）/ `standard`（標準・既定）/ `deep`（詳細）。

## ルート構成

| ルート | 役割 | 状態 |
|---|---|---|
| `/` | サイト一覧 | 既存を拡張（「サイトを登録」導線に差し替え） |
| `/sites/new` | ①サイト登録フォーム → 所有確認ガイド | 新規 |
| `/sites/[id]` | サイト別ダッシュボード | 既存を拡張（「新規スキャン」ボタン・所有未確認バナー追加） |
| `/sites/[id]/scan` | ②スキャン設定ウィザード | 新規 |
| `/scans/[id]` | ③進捗 →（ポーリング遷移）→ ④結果レポート | 新規 |

## 画面設計

### ① サイト登録 `/sites/new`

- フォーム: `name`（必須）/ `base_url`（必須・URL 形式）/ `verify_method`（`file`・`dns` 選択、既定 `file`）。
- `POST /sites` の結果で分岐:
  - `ownership_verified=true`（localhost/127.0.0.1/::1/*.local）: 「所有確認不要・登録完了」を表示し
    `/sites/[id]` へ誘導。
  - `false`: **所有確認ガイド**を図解表示（frontend ルール「テキストだけにしない」準拠）。
    - `verification.method='file'`: `file_path` にファイルを置き、中身に `file_content`（=トークン）を
      書く手順をステップ表示。パス・内容はコピー可能に。
    - `verification.method='dns'`: `dns_record`（TXT）を DNS に追加する手順をステップ表示。
    - 「確認する」ボタン → `POST /sites/:id/verify`。**422**→「まだ設置が確認できません」（再試行可）、
      **200**→ verified 表示 → 「スキャンへ進む」導線。
- コンポーネント: `SiteRegisterForm`（フォーム＋バリデーション表示）、`OwnershipGuide`
  （method で file/DNS のステップ図解を切替・コピー UI）。

### ② スキャン設定ウィザード `/sites/[id]/scan`

- **プリセット選択**: `light`/`standard`/`deep` のカード。各カードに説明（対象タグの広さ・所要時間の
  目安）。既定は `standard` を選択状態に。
- **危険パス除外**: デフォルト ON の**読み取り専用の安心表示**（「`logout`/`signout`/`delete` 等は
  自動で除外されます」）。トグルではない。
- 「スキャン開始」→ `POST /scans {site_id, preset}` → 202 の `scan_id` で `/scans/[id]` へ遷移。
- 前提: サイトが `ownership_verified=false` の場合はウィザードに入れず登録画面の確認へ誘導
  （backend も 403 で二重防御）。
- コンポーネント: `ScanPresetPicker`（プリセット定義は純粋 util・カード描画）。ウィザードの
  まとめ・開始アクションはページ `sites/[id]/scan.vue` がオーケストレーションする（薄い
  ラッパ component は作らない・YAGNI）。

### ③④ 進捗 → 結果 `/scans/[id]`（1ページ統合）

- **ポーリング** composable `useScanPolling(scanId)`:
  - `GET /scans/:id` を一定間隔（既定 2〜3 秒）で取得。
  - `queued`/`running` の間は継続、`done`/`failed` で停止。状態・最新 `ScanState`・停止フラグを返す。
  - アンマウント時にタイマ解除（リーク防止）。
- **進捗表示**（`status`≠done）: `queued → running → done` のステップ表示＋スピナー。`failed` は
  エラー表示（「スキャンに失敗しました」＋再実行導線）。
- **結果表示**（`status`=done）: 同ページを結果レポートに切替。
  - `summary`（`score`/`band`/`label`/`counts`）を大きく表示（既存 `ScoreCard` を再利用）。
  - `GET /scans/:id/findings` を取得し `FindingList` で重大度順に描画。各 `FindingCard`:
    title・`SeverityBadge`（重大度色）・URL・CWE・remediation。
  - findings 0 件: 「検出はありませんでした」。
  - 末尾に「専門家への相談」導線（v1 は静的プレースホルダのリンク/ボタン）。
- コンポーネント: `ScanProgress`、`FindingList`、`FindingCard`、`SeverityBadge`。

## 横断的関心事

### 重大度・スコアの色

- 重大度色（Critical/High/Medium/Low/Info）とスコアバンド色は **tokens.css の CSS 変数**へマップ
  （`--color-success`/`--color-warning`/`--color-m-red` 等）。既存 `utils/score-band.ts`・
  `utils/chart-palette.ts` の「実行時にトークンを解決」パターンを踏襲。生 hex 禁止。
- 重大度→トークンのマップは純粋 util（`utils/severity.ts` 等）に置き unit 100%。

### データ取得・エラー

- 取得は既存 `useApiClient()`（openapi-fetch 型付き）+ `useAsyncData({server:false})`。
- エラー表示は既存 `toApiErrorMessage`（`useApiError.ts`）に集約。フォーム/確認/スキャン開始の
  各エラー（400/403/409/422/500）を人間可読メッセージで提示。
- ミューテーション（`POST /sites`・`verify`・`POST /scans`）は `useAsyncData` でなく
  クライアント直呼び＋ローカル状態（送信中/成功/失敗）で扱う。

### スタイル・デザイン

- `.claude/rules/frontend.md` と `DESIGN.md` に準拠。`<script setup>` + Composition API、
  Tailwind セマンティッククラス（`on-dark`/`surface-card`/`hairline`/`muted` 等）。
- `<style>` ブロック・`style=""` インライン・生 hex は禁止。
- Mobile-first・レスポンシブ（固定 px 幅禁止）。

### テスト（Vitest・カバレッジ 100% ゲート）

- 純粋ロジック（プリセット定義、重大度→色、ポーリング状態機械、フォームバリデーション）は
  util/composable に切り出し、Statements/Branches/Functions **100%**。
- 各ページ・コンポーネントは `@nuxt/test-utils` + Vitest でレンダリング/分岐を網羅（既存 dashboard
  テストのパターンに合わせる）。API はモック。
- 既存の coverage 除外設定（`vitest.config.ts`）を踏襲。

## コンポーネント分割（責務）

```
pages/
  sites/new.vue          登録フロー（フォーム→ガイド→確認）のオーケストレーション
  sites/[id].vue         既存 + 「新規スキャン」導線・所有未確認バナー
  sites/[id]/scan.vue    ウィザードのオーケストレーション
  scans/[id].vue         進捗→結果のオーケストレーション
components/
  site/SiteRegisterForm.vue
  site/OwnershipGuide.vue
  scan/ScanPresetPicker.vue
  scan/ScanProgress.vue
  scan/FindingList.vue
  scan/FindingCard.vue
  scan/SeverityBadge.vue
composables/
  useScanPolling.ts
utils/
  scan-preset.ts         プリセット定義（表示名・説明・所要目安）純粋
  severity.ts            重大度→トークン/ラベル 純粋
```

## 段階化（実装プランの PR 分割）

- **PR-A**: `/sites/new`（`SiteRegisterForm`・`OwnershipGuide`）＋ `/` の登録導線。`POST /sites`・
  `POST /sites/:id/verify`。
- **PR-B**: `/sites/[id]/scan`（`ScanPresetPicker` + ページオーケストレーション）＋ `/sites/[id]` の「新規スキャン」導線・
  所有未確認バナー。`POST /scans`・`utils/scan-preset.ts`。
- **PR-C**: `/scans/[id]`（`useScanPolling`・`ScanProgress`・`FindingList`・`FindingCard`・
  `SeverityBadge`・`utils/severity.ts`）。`GET /scans/:id`・`GET /scans/:id/findings`。

各 PR は独立して `make test-web`（Vitest 100%）が通り、ブラウザで確認できる単位。PR-A→B→C の順に
一気通貫が広がる。

## 非対象（YAGNI）

- 認証持込 UI（`PUT /sites/:id/credentials`・マスク入力）— v2。
- 診断履歴画面（`GET /sites/:id/scans`）— v2。
- 危険パス除外の編集・設定画面 — engine デフォルト除外で足りる。
- SSE 進捗 — backend はポーリング前提。
- AI レポート平易化（BYOK）— フェーズ2。
