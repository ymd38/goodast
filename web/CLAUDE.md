@../CLAUDE.md
@../.claude/rules/frontend.md

# Web — 補足事項

## 責務
UI起点の全操作。サイト登録・スキャン設定ウィザード・レポート・履歴・ダッシュボード（Chart.js）。

## コマンド（web/ 内）

```bash
pnpm dev
pnpm build
pnpm test
pnpm lint --fix
pnpm type-check
```

変更後は必ず lint / type-check を実行してパスを確認。

## ディレクトリ構造

```
web/
├── assets/css/tokens.css   # デザイントークン（CSS変数の実体）
├── pages/                  # Nuxt ファイルベースルーティング
└── components/
    └── dashboard/          # Chart.js スコア推移グラフ
```

## ダッシュボード実装注意

- Chart.js は CDN ではなく `pnpm add chart.js` でインストールする
- グラフは `components/dashboard/` に集約し、ページコンポーネントから呼び出す
- ダークテーマ（`--color-canvas` = #000）で統一する
- 2回未満のスキャンでは遷移グラフを「データ不足」として空表示にする
