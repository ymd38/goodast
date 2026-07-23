# MEMORY.md — 意思決定ログ

> 議論を経て確定した決定・新発見・プランの見直しを時系列で記録する。
> フォーマルなADR（`docs/adr/`）を起こすほどでない粒度の「なぜそうしたか」はここへ。
> Claude Codeは実装前にここを参照し、過去の決定と矛盾しないことを確認する。

---

## 2026-06-28

### ADR-0001 実装: api/worker の2モジュール分離を確立

**実装**: `api/go.mod`・`worker/go.mod`・ルート `go.work` を作成し、別モジュール境界を物理的に確立した。

- module path: `github.com/ymd38/goodast/api` / `github.com/ymd38/goodast/worker`（リモート `ymd38/goodast` に一致）
- go directive: `1.26.5`（最新 1.26.x へ更新・2026-07-23。ローカルは `/usr/local/go` の compile が壊れ 1.24.0 のため `env -u GOROOT go ...` で module-cache toolchain を使う）
- `go.work` は `use ./api ./worker`（開発専用）
- 検証: 両モジュールで `go build` / `go vet` パス、`go list -m all` で2モジュール認識を確認
- 依存追加（Gin / river / Nuclei SDK）は未実施 — 各実装フェーズで個別に追加する。Nuclei SDK は `worker/go.mod` にのみ入れる

---

### ADR-0001 続き: 両プロセスの起動骨格を実装

**実装**: api / worker を独立起動できる実プロセスにし、Day-1 運用規約を組み込んだ。

- `{api,worker}/internal/config/` — 環境変数ロード＋起動時バリデーション（必須欠落・不正値で起動失敗）。table-driven テストで C0/C1 **100%**
- `api/cmd/api/main.go` — slog(JSON) / dig コンテナ / Gin(`/healthz`・`/readyz`) / pgxpool / SIGTERM・SIGINT で graceful shutdown
- `worker/cmd/worker/main.go` — slog / dig / std net/http のヘルスサーバ(:9090) / pgxpool / graceful shutdown
- dig は struct-based injection（`dig.In`）、コンテナ構築は `cmd/` のみ
- 依存追加: api=gin/dig/pgx・v5、worker=dig/pgx・v5（**worker に gin は入れない**）
- `/readyz` は pgxpool の遅延接続を利用し、DB停止中でも起動可・503返却。スモークテストで healthz=200 / readyz=503 / graceful shutdown(exit 0) を確認

**注意**: river のジョブ消費ループ（worker serve 内に NOTE コメント）は ADR-0005、Nuclei SDK は ADR-0002、sqlc/マイグレーションは別タスクで追加する。

---

### 設計原則: プリミティブ依存の排除・カプセル化

**指示（ユーザー）**: プリミティブ型に依存せず、データをカプセル化（Go では struct / named type）し、データ操作ロジックを分散させない（Tell, Don't Ask）。

**How to apply**: 意味を持つ概念（Score / Severity / TargetURL / VerifyToken / EncryptedHeaders / 各種ID）は専用型に包み、コンストラクタで不変条件を検証、操作はメソッドに同居させる。IDは型付けして引数取り違えをコンパイル時に防ぐ。詳細は `.claude/rules/backend.md` の「ドメイン型とカプセル化」。

**境界**: sqlc struct は永続化境界のデータキャリア。repository が sqlc row ↔ ドメイン型を変換する唯一の箇所。不変条件・振る舞いを持たない単純データまで機械的に包まない（YAGNIと両立）。

---

### アーキテクチャ方針: package-by-feature（クリーンアーキは不採用）

**決定**: フルのクリーンアーキテクチャ（domain/usecases/services/infrastructure の最上位レイヤー分割）は採用せず、package-by-feature + feature内の薄い層分けを採用する。

**理由**: クリーンアーキは既存決定3つと衝突する。(1) sqlc — domain entity独立を要求しsqlc structの詰め替えマッピングが大量発生、(2) YAGNI（インターフェースは2実装以上のみ）— クリーンアーキは1実装でもrepository/usecase interfaceを必須化、(3) 実DBテスト（testcontainers）— repository抽象化の最大の動機（モック化）が消える。さらにGo主流はpackage-by-layerではなくpackage-by-feature。

**How to apply**: `site/` `scan/` `report/` 等をfeatureとして切り、内部を `service.go`（gin非依存のビジネスロジック）/ `repository.go`（sqlcラッパー）/ ドメイン純粋関数 / `types.go`（乖離時のみ）に分割。依存方向は「handler→serviceのみ」「serviceはgin/net/httpをimportしない」の2ルール。詳細は `.claude/rules/backend.md` の「アーキテクチャ方針」。

**例外**: `worker/internal/engine/` はフェーズ2のZAP追加（Nuclei/ZAPの2実装）を見据え `Engine` インターフェースを切る。

---

### アーキテクチャ方針: PoC = 本番品質で実装

**決定**: PoC成功確度が高いため、最初から本番用アーキテクチャで実装する。「後で書き直す前提のPoC品質」は取らない。

**スコープは変わらない**（Phase 1の機能範囲）。変わるのは実装品質：
- 構造化ログ（slog）、Graceful shutdown、ヘルスチェック をday1から実装
- 12-factor準拠の設定管理（環境変数・起動時バリデーション）
- pgxpoolの明示的チューニング
- multi-stage Dockerfile / non-rootユーザー

**How to apply**: `.claude/rules/backend.md` の「本番運用規約」セクションを参照。ショートカットを取る実装提案があればブロックする。

---

### デザイントークンの橋渡し構造

**決定**: `design-tokens.md` を廃止し `.claude/rules/frontend.md` に統合。

- `.claude/rules/frontend.md` — コーディング規約 + セマンティックトークンルール（スコア色分け・禁止事項・MTricolor等）
- `web/assets/css/tokens.css` — ブラウザが読むCSS custom properties（値の正）

**理由**: `design-tokens.md` の変数名対応表は `tokens.css` を読めば導出できる（重複）。セマンティックルールは `frontend.md` に統合して参照先を1ファイルに集約する方が管理しやすい。

---

### ディレクトリ構成の確定

`docs/poc-plan.md §9` の想定構成をそのまま採用し初期スキャフォールドを作成。

追加した点: `web/assets/css/` を poc-plan.md の想定に加えて作成（トークンCSS置き場として必要）。

---

### migiudedirect-beta からの rules/agents 移植

migiudedirect-beta の `.claude/` 構成を参照し、以下を goodast 向けに適合させて配置した。

**採用・適合したもの:**
- `.claude/rules/git.md` — ブランチ命名・Conventional Commits・PRテンプレート（scope を goodast 用語に変更）
- `.claude/rules/backend.md` — Go/Gin ルール（GORM→pgx、pgvector除外、river・Nuclei制約を追加）
- `.claude/rules/frontend.md` — Nuxt ルール（Nuxt4→Nuxt3、Storybook除外、DESIGN.md参照に変更）
- `.claude/agents/issue-to-pr.md` — Issue→PRオーケストレーター（goodast 固有のセキュリティゲートを追加）
- `api/CLAUDE.md` / `worker/CLAUDE.md` / `web/CLAUDE.md` — ディレクトリ別補足（`@`インポート方式）
- ルート `CLAUDE.md` — Plan First規約・タスク別参照ファイル表を追記

**スキップしたもの（理由）:**
- `ui-design.md` / `design-principles.md` — goodast は DESIGN.md（BMW M デザインシステム）が代替
- `components/*.md` — UI実装前のため不要。コンポーネント実装時に随時追加
- `nuclei-scan` スキル — goodast 自体がNucleiスキャナーなので役割が逆

### DBアクセス層: sqlc を採用

**決定**: pgx 直接クエリ（生SQL）をやめ、sqlc を採用する。

**理由**: goodast はセキュリティアプリケーションであり、生SQLをGoコードに直書きする方式は開発者の規律に依存するため不適切。sqlc は SQL を `queries/*.sql` に記述しコンパイル時に型安全なGoコードを生成する仕組みで、SQLインジェクションが構造的に不可能。動的文字列結合のような逃げ道が生成コードに存在しない。

**How to apply**: DBクエリは必ず `api/internal/db/queries/` または `worker/internal/db/queries/` の `.sql` ファイルに記述し `sqlc generate` で生成する。Goコードへの生SQL直書きは禁止。

---

### ADR-0002 着手: Nuclei エンジン統合の設計判断

**スコープを未認証スキャンに限定**: session 認証（Cookie/Bearer 持込）の復号・ヘッダ注入は **ADR-0003（アプリ層暗号化）へ分離**。`enc_headers` 復号が ADR-0003 依存のため。`engine.ScanRequest` は将来ヘッダ受け口を足せる形にし、worker は当面 unauthenticated スキャンのみ実行する。

**engine を「純粋層 + SDKアダプタ」に2分割**:
- `worker/internal/engine/`（純粋・SDK非依存）= interface・スコープ allowlist・危険パス除外・severity 正規化・集計。**unit 100%**。
- `worker/internal/engine/nuclei/`（Nuclei SDK 隔離）= 薄いグルー。ネットワーク＋テンプレート必須でユニットテスト不可のため `//go:build integration` で検証し、**coverage 計測から除外**（`backend.md` の除外リストに追記）。
- **理由**: 「DB/SDK 結合コードは integration 網羅、純粋ロジックは unit 100%」という既存 `scanjob` の方針に整合。100% C0 要件と SDK の現実的非テスト性を両立する。

**並行安全性 = スキャンごとに NucleiEngine を生成・破棄**: river `MaxWorkers=5` で並行実行されるため、`LoadTargets`/実行の共有状態を持たないよう per-scan で生成。テンプレート再ロードのコストは PoC では許容。

**sandbox は `WithSandboxOptions(false, false)`**: ローカルファイル読取は禁止（過去の Nuclei LFI/RCE テンプレートリスク対策）。ただし **localnetwork 制限はしない**（localhost / Juice Shop 等の自前ターゲットを scope allowlist で許可するため）。スコープ逸脱防止は自前の `Scope.Allows` で担保。

**`engine.Engine` に `Version()` を追加**: `scans.engine_version` にエンジン識別子＋固定版（`nuclei/v3.9.0`）を記録するため。将来 ZAP も版を持つのでインターフェースに自然に乗る。Nuclei 版は go.mod と `nuclei.go` の定数の2箇所を同期させる。

**worker 側 defense-in-depth 所有確認（ADR-0004）**: api の受付ゲート（PR#3 S1）に加え、worker でも `GetScanTarget` 取得後に再チェック。未確認 public ホストは `FailScan`。api と同じ localhost/127.0.0.1/::1/*.local 例外（別モジュールのため `Scope` に再実装）。

---

### ADR-0002 PR レビュー（Qodo）対応の判断

**リクエスト時の host/path 強制は SDK で安全に注入できない（#2/#6 持ち越しの根拠）**: Nuclei lib には
`Scope`/`OutOfScope`/redirect ポリシーを渡す専用オプションが無く、唯一の入口 `WithOptions(*types.Options)`
は `e.opts` を**丸ごと置換**して他オプションの既定を壊す（版依存で脆い）。よって per-request の
host/path allowlist は実装せず、(a) テンプレート選択での破壊的タグ除外（`dos`+`intrusive`）、
(b) 検出結果の `scope.Allows` フィルタ（defense-in-depth）で対応。現状は単一ターゲット・非クロール
（katana 無効）・DAST fuzzing 無効のため逸脱の主経路はテンプレートの固定パス要求とクロスホスト
redirect に限定。**リクエスト時の厳密遮断はクロール/認証スキャン導入フェーズ**（カスタム transport が
必要十分になる時）に実装する。安易な `WithOptions` ハックは禁止。

**severity は「そのまま使用」=分類を変えない、であって大小文字までは固定でない（#1 反論）**: DB CHECK は
`Critical/High/Medium/Low/Info`。Nuclei は小文字 `critical/...` を返すため大小文字正規化は必須で、値の
分類は 1:1 保持（critical→Critical）。`unknown`/空のみ schema に値が無いため `Info` に安全コアース。
これは再分類（High→Medium 等）ではないので「テンプレ severity をそのまま使用」に違反しない。

**scan の終端確定（#4/#5/#7）**: StartScan で running にした後、(1) 設定不備（不正URL・所有未確認）は
`markFailed` で failed 確定、(2) engine 実行エラーは原則 river 再試行に委ね、最終試行
（`job.Attempt>=job.MaxAttempts`）で失敗したら failed 確定、(3) `FailScan` の DB 更新自体に失敗したら
error を返して running 放置を防ぐ。再試行時の findings 重複は実行前 `DeleteFindingsByScan` で掃除して冪等化。

---

### ADR-0004 site feature（サイト登録＋所有確認）の設計判断

**ローカル判定を共有パッケージ `api/internal/target/` に集約（DRY・セキュリティ境界）**: 「所有確認をスキップしてよいか」は
scan（受付ゲート）と site（登録・確認）の双方で必要で、二重化するとドリフト＝安全性のほころびになる。
`IsLocalTarget` / `RequiresOwnershipVerification` を target に一元化し、scan の私有コピーを撤去して寄せた。
worker 側（`engine.Scope`）は別モジュールのため共有できず別実装（同ロジック）を維持。

**所有確認の外部 I/O は注入で抽象化（テスト容易性）**: `Verifier` は `httpDoer`/`dnsResolver`（いずれも狭い
非公開インターフェース）を注入。本番は `*http.Client`/`net.DefaultResolver`、テストは fake。file 方式は
`WithContext` でリクエスト構築し `NewRequestWithContext` の到達不能エラー分岐を排除、`crypto/rand` は
`randRead` 変数で差し替え可能にして純粋層 unit 100% を達成。

**ローカルサイトは登録時に token/method を持たせない**: ローカル（localhost 等）は確認不要のため
`verify_method`/`verify_token` を NULL 保存。`POST /verify` では確認不要と判定して即 `ownership_verified=true`
にする（UI 表示の一貫性のため verified を立てる。scan 受付ゲートは元々ローカルをスキップ）。

**所有確認トークンは秘匿対象でない**: ユーザーが自サーバに設置/公開する検証値のため、API レスポンスで
返してよい（認証情報 Cookie/Bearer のマスク要件〈ADR-0003〉とは別扱い）。

**HTTP ステータス対応**: 同名重複=409 / 不在=404 / 所有確認未達（ファイル未設置・TXT不一致）=422 / 入力不正=400。

---

### UI一気通貫フロー v1（frontend）実装で確定した規約・教訓（2026-07-07）

**ネストコンポーネントの auto-import 解決は mountSuspended テストで実証する（標準規約化）**: Nuxt は
ディレクトリプレフィックス（`components/scan/SeverityBadge.vue` → `<ScanSeverityBadge>`）で解決する。
テストで `global.components` に手動登録すると未解決タグが隠蔽され「テスト green・本番は空描画」になる
（C3 で実際に発生）。以後、入れ子コンポーネントを持つ各コンポーネントに mountSuspended による
実解決テストを1本置く（`FindingList.spec.ts` が参照実装）。

**swagger 生成型は全フィールド optional — 検証は lint だけでなく `pnpm type-check` を必須にする**:
`severity`/`scan_id` 等の optional を握り損ねた型エラーは vitest では検出できない（C1 で lint+test 通過・
type-check 失敗が発生）。frontend 変更の検証トリオは test / lint / type-check。

**useScanPolling の残課題（必要になったら）**: 現状 `stopped` フラグで stop 中 in-flight tick の再アームは
防いでいるが、「GET 実行中の再 start()」は旧 tick が復活し得る（unmount では全チェーン停止するので実害なし・
mount-once 運用では到達不能）。リトライボタン等で再 start が入る UI を作る際は generation counter 方式に置き換える。

**a11y follow-up（ErrorNotice 抽出と同時に対応）**: エラーバンド（`border-m-red` の `<p>` が6箇所に重複）を
`ErrorNotice` atom に抽出するタイミングで `role="alert"`/`aria-live` を持たせる。プリセットカードの
`aria-pressed` も同じ a11y バッチで。

---

### Tailwind v4: 命名スペーシングトークンと Tシャツサイズ系ユーティリティの名前衝突（2026-07-07・手動E2Eで発覚）

`tokens.css` の `@theme` は `--spacing-xs/sm/md/lg/xl/xxl` を命名スペーシングとして定義している。これが
Tailwind v4 の Tシャツサイズ系ユーティリティ（`max-w-xl` 等）と**キー名で衝突**し、`max-w-xl` が本来の
`--container-xl`(36rem) でなく `--spacing-xl`(40px) に解決される。結果 `/sites/new` の `max-w-xl` が幅40pxに
潰れ、文字が縦積みになった（`max-w-3xl` は spacing に `3xl` が無いので無事）。

**Why:** jsdom の Vitest はレイアウトを評価しないためテストで検出できず、コードレビューの「クラスがトークンに
対応するか」監査も『別の（誤った）トークンに解決される』ことは見抜けない。**実ブラウザ（computed style）確認が必須**。

**How to apply:** `max-w-<name>` / `w-<name>` 等で spacing トークンと同名（xs/sm/md/lg/xl/xxl）のキーを使わない。
コンテナ幅は `max-w-page`（定義済み）か spacing と衝突しない `2xl/3xl` を使う。恒久対策としては spacing トークンを
数値スケールか別名（例 `--spacing-space-*`）に寄せる案があるが、tokens.css 変更は設計判断（frontend rules）。
関連: [[frontend-uxflow-pending]]。
---

### Nuxt ルーティング: [id].vue と [id]/scan.vue の親子シャドウ（2026-07-08・手動E2Eで発覚）

`pages/sites/[id].vue`（詳細）と `pages/sites/[id]/scan.vue`（ウィザード）が共存すると、Nuxt は `[id].vue` を
親・`[id]/` 配下を子ルートにする。親に `<NuxtPage/>` が無いと `/sites/:id/scan` は**子（ウィザード）を描画せず親（詳細）が出る**。
症状は「新規スキャンを押しても画面が変わらない」。修正は `[id].vue` → `[id]/index.vue` に改名して兄弟ルート化。

**Why:** jsdom の Vitest は各ページを直接 import してマウントするためルート解決を通らず、この衝突を検出できない。
実ブラウザ or ランタイムの `$router.getRoutes()` でしか見えない。

**How to apply:** 動的セグメント配下にネスト画面を足すときは最初から `[id]/index.vue` + `[id]/child.vue` で切る。
関連: [[frontend-uxflow-pending]]。同 E2E で判明した [[tailwind-spacing-token-collision があれば]] 系と併せ、frontend は実ブラウザ確認を必須に。
<!-- 新しいエントリは上の区切り線の上に追加する -->
