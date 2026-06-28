# multi-terminals v0.2.0

複数のターミナルを 1 画面でレイアウト管理し、フォルダ・起動コマンド・配置を保存して翌日に再現できるアプリです。UI を組み込んだ**単一実行ファイル**（サーバ版）と、ブラウザ不要の **Wails デスクトップ版**の 2 形態を提供します。別途 Node/Vite は不要です。

## 配布物

### サーバ版（単一バイナリ／UI 組み込み）
起動後、ブラウザで **http://localhost:8080** を開きます。

- Windows: `multi-terminals-windows-amd64.exe` / `multi-terminals-windows-arm64.exe`
- macOS: `multi-terminals-darwin-arm64`（Apple Silicon）/ `multi-terminals-darwin-amd64`（Intel）/ `multi-terminals-darwin-universal`

### デスクトップ版（Wails）
ブラウザ不要・ネットワークポート不使用で起動するネイティブアプリです。

- macOS: `multi-terminals-darwin.app.zip`（展開して `multi-terminals.app` を実行）
- Windows: デスクトップ版 `.exe` は GitHub Actions（ネイティブランナー）でビルドします。

## 起動と設定

サーバ版はポートやデータ保存先を環境変数で変更できます:

```
# Windows (PowerShell/cmd)
set PORT=9000
set MULTI_TERMINALS_DIR=C:\Users\you\mt-data
set MULTI_TERMINALS_SHELL=cmd.exe   :: 既定は powershell.exe

# macOS
PORT=9000 MULTI_TERMINALS_DIR=~/mt-data ./multi-terminals-darwin-arm64
```

## v0.2.0 の主な変更

- **Wails デスクトップ版**を追加（既存 UI/サーバを再利用、端末 I/O はネイティブバインディング）。
- **全アプリ一括ビルド**と各種ビルドスクリプトを整備（`scripts/build-all.sh` / `build-mac.sh` / `build-wails.sh`）。Windows/macOS 両対応の GitHub Actions も追加。
- **左サイドバーの折りたたみ**（状態を保存）。
- **ワークスペース（ビュー）の削除**（起動中セッションも閉じる）。
- **Ctrl+Shift+矢印 でペイン間フォーカス移動**。
- **各ペインにタイトルを付与**（ヘッダー表示・インライン編集・保存して再現。未設定時はディレクトリ表示）。

## 既存機能

- 端末は OS ネイティブの PTY で動作（Windows は ConPTY、既定シェル PowerShell）。
- 1画面 / 左右2分割 / 上下2分割 / 4分割 + アクティブペイン最大化。
- フォルダ・起動コマンド(自動実行/手動)・レイアウト・タイトルを保存して翌日に再現。
- セッションはサーバー側で保持。閉じても継続し、再オープンでスクロールバック復元。

## 注意

- すべて **未署名**です。初回起動時に Gatekeeper（macOS）/ SmartScreen（Windows）の警告が出る場合があります（macOS は右クリック→開く、Windows は「詳細情報」→「実行」）。
- 配布物は macOS 上でビルド/検証しています。Windows 実機での動作確認を推奨します。問題があれば Issue へ。

## チェックサム (SHA-256)

`SHA256SUMS.txt` を参照してください。
