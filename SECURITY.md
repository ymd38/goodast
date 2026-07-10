# Security Policy / セキュリティポリシー

> English summary: goodast is a DAST (web vulnerability scanner) intended **only for
> targets you own or are explicitly authorized to test**. To report a vulnerability
> in goodast itself, please use
> [GitHub Private Vulnerability Reporting](https://github.com/ymd38/goodast/security/advisories/new)
> — do **not** open a public issue.

## 脆弱性の報告方法

goodast 自体の脆弱性を発見した場合は、**公開 Issue を立てず**、以下の窓口へ非公開で報告してください。

- [GitHub Private Vulnerability Reporting](https://github.com/ymd38/goodast/security/advisories/new)（推奨）

報告には可能な範囲で以下を含めてください。

- 影響を受けるコンポーネント（`api` / `worker` / `web`）とバージョン（コミットハッシュ可）
- 再現手順（PoC があれば添付）
- 想定される影響（例: スコープ外ドメインへのリクエスト逸脱、認証情報の平文露出 等）

受領後は以下の目安で対応します。

| 対応 | 目安 |
|---|---|
| 受領確認 | 7 日以内 |
| 初期評価（影響度判定） | 14 日以内 |
| 修正・アドバイザリ公開 | 影響度に応じて順次 |

## 対応バージョン

PoC 段階のため、セキュリティ修正は **`main` ブランチの最新版** に対してのみ提供します。
タグ付きリリース開始後は、サポート対象バージョンをこの表で明示します。

| バージョン | サポート |
|---|---|
| `main`（最新） | ✅ |

## 責任ある利用（Responsible Use）

goodast は動的脆弱性検査（DAST）ツールであり、対象サイトへ実際にリクエストを送信します。
**必ず自分が所有するサイト、または明示的に検査の許可を得たサイトに対してのみ使用してください。**
許可のない第三者のサイトへのスキャンは、不正アクセス禁止法等の法令に違反する可能性があります。

goodast は誤用を防ぐため、以下のガードレールを実装しています（詳細は `docs/adr/`）。

- **ドメイン所有確認**: 所有確認（ファイル設置 / DNS TXT）を通すまで外部ドメインへのスキャンは実行できない（ADR-0004）
- **スコープ allowlist**: スキャン対象は登録ドメインに限定し、外部ドメインへの逸脱を禁止
- **危険パスのデフォルト除外**: `logout` / `delete` / `admin/*` 等、破壊的操作を踏み得るパスを除外
- **保守的なデフォルトレート**: 外部対象への同時リクエストは低レートに制限
- **破壊的テンプレートのデフォルト無効**: `dos` / `intrusive` タグのテンプレートは実行しない

これらのガードレールの回避方法に関する質問・機能要望には対応しません。
ガードレール自体の欠陥（バイパス可能な実装不備など）は、上記の窓口へ**脆弱性として**報告してください。
