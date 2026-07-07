# クロール・ガードレール方針（Phase 2）設計

> ステータス: **方針 spec（実装なし）**。Phase 2 でクロール（対象URL探索・拡張）を導入する際に
> 従うガードレールとアーキテクチャ方向を定める。あわせて、現 PoC（Phase 1）がこの将来像を
> 阻害しないための前方互換制約を明記する。実装は本 spec 承認後に別途プラン化する。

作成: 2026-07-08

## 1. 背景と非目標

### 背景
現 PoC は**単一URL・非クロール**（nuclei の katana 無効・DAST fuzzing 無効）でスキャンする。
スコープ逸脱の主経路が限定的なため、スコープ強制は post-filter（`engine.Scope.Allows` による
結果フィルタ）で足りている（ADR-0002 backlog 参照）。

Phase 2 でクロールを入れると、探索器がリンクを辿って自ら fetch し、対象URLが単一から多数へ増える。
これにより以下が顕在化する:
- **スコープ逸脱**が post-filter では防ぎきれない（fetch 自体が起きてしまう）。
- **危険パス**（logout/delete/admin 等）を探索が踏むリスク。
- **状態変化**（特に認証状態でのフォーム送信・破壊的リンク追従）。
- **固定タイムアウトが対象URL数でスケールしない**（所要 ≈ 対象URL数 × テンプレ数 ÷ レート）。

### 非目標（この spec では扱わない）
- クロールエンジンの選定（nuclei-katana か ZAP か）… **別 ADR** とする。本 spec は
  どちらでもガードが成立する**エンジン非依存**の枠組みを定める。
- AI 駆動・タスク駆動クロール（poc-plan Phase 3）。
- フォームのヒューリスティック分類による「安全そうな破壊的操作」の実行（採用しない・YAGNI/安全第一）。

## 2. 用語: クロール段とスキャン段の分離（最重要）

本設計は 2 つのフェーズを明確に分ける。**「GET-only」等の制約はクロール段にのみ適用し、
スキャン段には適用しない**（診断で POST を送れなければ DAST として機能しないため）。

| フェーズ | 目的 | HTTP method | 副作用 |
|---|---|---|---|
| **クロール段** | リンク・**フォームの発見**（探索） | **GET のみ** | 状態を変えない（フォーム送信しない） |
| **スキャン段** | 発見した対象への**脆弱性診断** | テンプレ/ファジングが要する任意 method（POST 等を含む） | 非破壊のテストデータ生成は許容 |

- クロール段は GET でナビゲーションしつつ、**フォームを抽出**する（action URL・method・
  フィールド名を診断対象として記録するのみ・**送信はしない**）。→ POST エンドポイントを
  「発見」するが探索中は叩かない。
- スキャン段は、発見した**非危険**エンドポイントに対して次の 2 種の POST を実行する:
  1. **テンプレ駆動 POST**（nuclei テンプレが固定パスへペイロードを POST する類。既存・抑制しない）。
  2. **フォーム/パラメータ・ファジング**（抽出したフォームのフィールドにペイロードを注入して
     送信。現状「DAST fuzzing 無効」で止めているものを Phase 2 で有効化）。アプリ固有入力の
     SQLi/XSS 検出はこれが担う。

### 制約の精緻化
「認証後スキャンで状態変化回避」は **「破壊的・不可逆な状態変化を避ける」** に読み替える。
全 POST を止めるのではなく、破壊的操作のみを多層で遮断する（§4）。非破壊のテストデータ生成
（検索・コメント等への試験送信）は検出のために許容し、UI で事前に警告する。

## 3. アーキテクチャ: 二段構え＋共有 ScopedTransport（採用案 A）

スキャンジョブ内で **クロール段 → タイムアウト算出 → スキャン段** を実行する。動的タイムアウトは
対象URL数を要するため、この順序（先に数え、それから時間枠を決める）が必然となる。

```
[worker scan job]
  crawl(engine, ScopedTransport, bounds) ──▶ URL集合 + 抽出フォーム
        │
        ▼  timeout = clamp(base + |URLs|×avgReqPerTmpl×|tmpl| / rate, floor, CEILING)
  ctx' = context.WithTimeout(ctx, timeout)          // river job Timeout=CEILING、内側で動的値
        │
        ▼
  scan(nuclei, ScopedTransport, ctx') ─────▶ findings
```

### 3.1 ScopedTransport（ガードの単一集約点）
engine 層に新設する `http.RoundTripper`（または同等のリクエスト介入層）。**クロール器と nuclei の
両方が同一インスタンスを使う**。リクエスト時に次を強制する:

- **host allowlist**: `engine.Scope.Allows(url)`（host:port 一致）を満たさない宛先へは
  **リクエストを発行しない**。allowlist は登録ドメイン（+任意サブパス）を正とする。
- **危険パス遡断**: `logout` / `signout` / `delete` / `remove` / `destroy` / `admin/*` 等
  （既存の危険パス定義を正とする）へのリクエストを遮断。
- **クロール段の method 制限**: クロール段では GET 以外を遮断（フォーム送信・副作用 method 禁止）。
  スキャン段ではこの制限を課さない（テンプレ/ファジングが POST 等を要するため）。
- **cross-host redirect 追従禁止**: 別ホストへの 3xx を追わない（現 W3 の `DisableRedirects` を
  クロールにも一般化）。認証ヘッダの別ホスト送出を防ぐ。

> スコープ強制を **post-filter からリクエスト時遡断へ格上げ**する（ADR-0002 の持ち越しを解消）。
> 逸脱が「物理的に」起きない状態を作る。

### 3.2 エンジン非依存
クロール器は `Crawler` インターフェース裏に置き、ScopedTransport を注入する。katana でも ZAP でも
ガードは不変。エンジン選定は別 ADR で決め、本枠組みの内側で差し替える。

## 4. 状態変化の多層防御（破壊的操作の遮断）

スキャン段が POST を送れる一方で、破壊的操作は次の多層で遮断する:

1. **危険パス遡断**（§3.1）: logout/delete/admin/* 等は ScopedTransport が遮断 → そもそも POST しない。
2. **破壊的テンプレ除外**（既存）: `dos` / `intrusive` タグはデフォルト無効・明示オプトインのみ。
3. **ScopedTransport のスコープ強制**（§3.1）: allowlist 外・cross-host へは出さない。
4. **UI 明示警告**: 能動スキャンは対象に非破壊のテストデータ等を作り得る旨を初心者向けに事前提示し、
   合意の上で実行する。

認証クロールはこれらのガード下で許可する（GET-only クロール＋危険パス遡断により、認証状態でも
状態を変えない）。セッションヘッダは ADR-0003 経路で注入し、ScopedTransport が cross-host 送出を防ぐ。

## 5. クロール有界化と動的タイムアウト

### 5.1 クロール上限（安全シーリング・preset 毎）
動的タイムアウトを採用しても、暴走（青天井）を防ぐため**クロール自体の上限は必須**とする:
- **max URL 数**（発見URLの打ち切り上限）
- **max 深さ**（リンク追跡の段数上限）
- **同一ホスト限定**（§3.1 の allowlist で担保）

preset 毎の方針デフォルト（値は調整可能）:

| preset | クロール | 目安上限 |
|---|---|---|
| light | なし（単一URL・現状） | — |
| standard | 浅い | max 50 URL / depth 2 |
| deep | 広い | max 200 URL / depth 3 |

### 5.2 動的タイムアウト
クロール後に確定した対象URL数から算出する:

```
timeout = clamp(base + |URLs| × avgReqPerTemplate × |templates| / rate, floor, CEILING)
```

- `rate` は既存のレートモデル（`ScanProfile` の RateLimit、ローカル対象は `ForLocalTarget`）を消費する。
- **CEILING（preset 毎の絶対上限）を backstop** とする。動的値が壊れても CEILING を超えない。
- **配線**: river の `Timeout()` コールバックはクロール前に呼ばれ URL 数を持てないため、
  **river ジョブ Timeout = CEILING** とし、クロール後に **内側で `context.WithTimeout(ctx, 動的値)`**
  を適用する。ジョブは 1 本のまま二段を実行する。

## 6. 前方互換: 現 PoC が阻害してはいけないこと

Phase 1 実装は次を保ち、Phase 2 の土台を壊さないこと:
- `engine.Scope`（`Allows` / host 正規化）を allowlist の**唯一の正**として維持する
  （ScopedTransport が再利用する）。危険パス定義も一元管理を維持する。
- `engine.Engine` インターフェース境界を保ち、`Crawler` を並置できる形を維持する。
- レートの parametrize（`ForLocalTarget` 等）を維持する（動的タイムアウトが消費する）。
- 認証ヘッダ注入（ADR-0003）と cross-host 遮断（W3）の実装を、ScopedTransport へ一般化できる形で残す。

## 7. テスト方針

- **ScopedTransport**: 純粋 unit（許可/遮断マトリクス＝ host・危険パス・method・cross-host redirect の
  全分岐）100%。ネットワーク不要。
- **動的タイムアウト式**: 純粋 unit（clamp の floor/CEILING 境界、URL=0 / 大規模の両端）。
- **クロール有界化ロジック**（上限適用・同一ホスト判定）: 純粋 unit 100%。
- **クロール結合**（実エンジン・実 fetch）: `//go:build integration`（nuclei アダプタと同様に
  カバレッジ計測から除外し integration で検証）。
- **フォームファジングの状態変化安全性**: 検証環境（Juice Shop）で危険パスを踏まないこと・
  allowlist 外へ出ないことを integration で実証する。

## 8. 決定事項サマリ

| 論点 | 決定 |
|---|---|
| ゴール | 将来向けガードレール方針（実装なし）。現 PoC の前方互換制約も定める |
| クロール段 | GET-only ナビゲーション＋フォーム抽出（送信しない） |
| スキャン段 | 非危険エンドポイントへ POST 含めて診断（テンプレPOST＋フォームファジング） |
| スコープ強制 | post-filter → **リクエスト時遡断**（ScopedTransport）へ格上げ |
| 状態変化 | 「破壊的・不可逆」のみ多層遮断。非破壊テストデータは許容＋UI 警告 |
| 認証クロール | GET-only＋危険パス遡断の下で**許可** |
| ワークロード/時間 | **動的タイムアウト**（対象数連動）＋ preset 毎の**絶対 CEILING**＋クロール上限 |
| アーキ | 二段構え（crawl→算出→scan）＋共有 ScopedTransport。エンジン非依存 |
| エンジン選定 | 別 ADR（katana / ZAP）。本枠組みの内側で差し替え |

## 9. 未決定（実装プラン化・後続 ADR で詰める）

- クロールエンジンの選定（nuclei-katana / ZAP）と、それに伴う運用・配布の増分。
- `avgReqPerTemplate` 係数・`base` / `floor` / `CEILING` の具体値（実測でチューニング）。
- フォーム抽出のデータ形（フィールド型・CSRF トークン取り回し）とファジングのペイロードセット。
- クロールの robots.txt / rate 尊重ポリシー（外部対象向け）。
