# サーバ待ち受け(bind アドレス・ポート)設定 — 設計

日付: 2026-07-27
対象: `apps/web/cmd`(Go バックエンドの起動部)

## 背景 / 目的

現状、待ち受けポートは環境変数 `PORT`(既定 8080)で変更できるが、bind アドレスは
`":" + port` にハードコードされており常に全インターフェースで待ち受ける。
bind アドレスとポートの両方を `.env` ファイルで設定できるようにする。

## 要件

- `.env` ファイルで `HOST` と `PORT` を設定できる。
  - `HOST`: bind アドレス。既定は空文字(= 全インターフェース)。例: `127.0.0.1`
  - `PORT`: 待ち受けポート。既定は `8080`。
- `.env` の置き場所は **サーバ起動時のカレントディレクトリ**(`./.env`)。
- 優先順位: **プロセス環境変数 > `.env` > 既定値**(dotenv の一般的な慣習)。
  既存の `PORT` 環境変数の挙動は維持される。
- `.env` が存在しない場合は黙ってスキップ。存在するが読めない場合は警告ログを出して続行。
- 起動ログに実際の bind アドレス(`HOST:PORT` 形式)を表示する。

## アプローチ

外部ライブラリ(godotenv 等)は導入せず、標準ライブラリのみの小さなパーサを
`apps/web/cmd` 配下に実装する。理由: このプロジェクトは外部依存を最小限に隔離する
方針であり、必要なのは単純な `KEY=VALUE` の読み取りのみのため。

### .env のサポート構文

- `KEY=VALUE`(VALUE は行末まで。前後の空白はトリム)
- 先頭 `#` のコメント行、空行は無視
- 任意の `export ` プレフィックスを許容
- `"..."` / `'...'` で囲まれた値はクォートを除去
- 上記以外の行(`=` を含まない等)は無視(警告ログ)

## コンポーネント

- `apps/web/cmd/env.go`(新規)
  - `loadDotEnv(path string) (map[string]string, error)` — .env をパースして map を返す
  - `resolveAddr(getenv func(string) string, dotenv map[string]string) string`
    — 優先順位を適用して `host:port` を組み立てる
- `apps/web/cmd/main.go`(変更)
  - 起動時に `./.env` を読み、`resolveAddr` の結果を `http.Server.Addr` に使う
  - 既存の `portFromEnv` は `resolveAddr` に統合して削除

## エラー処理

- `.env` 不在: 正常系。何もしない。
- `.env` 読み取りエラー: `log.Printf` で警告して既定値で続行。
- 不正なアドレス(不正な HOST/PORT 値): `ListenAndServe` のエラーとして表面化し、
  既存の `log.Fatalf` で終了する。追加のバリデーションは行わない(YAGNI)。

## テスト

`apps/web/cmd/env_test.go`(新規):

- パーサ: 基本の KEY=VALUE / コメント / 空行 / export / クォート / 不正行の無視
- `resolveAddr`: 既定値 / .env のみ / 環境変数が .env より優先 / HOST+PORT の組み合わせ

## ドキュメント

README の起動説明に `.env`(HOST/PORT)の説明を追記する。

## スコープ外

- フロントエンド(Vite)側の接続先は既存の `VITE_API_TARGET` で対応済みのため変更しない。
- UI からの設定変更、settings.json への統合は行わない。
