multi-terminals の Windows 向け初回プレリリースです。フロントエンド(UI)を組み込んだ**単一の実行ファイル**で、別途 Node/Vite は不要です。

## ダウンロードと起動

1. お使いの CPU に合わせて `.exe` をダウンロード:
   - 一般的な Intel/AMD PC → **multi-terminals-windows-amd64.exe**
   - ARM 版 Windows → **multi-terminals-windows-arm64.exe**
2. ダウンロードした `.exe` を実行（コンソールが開きます）。
3. ブラウザで **http://localhost:8080** を開く。

ポートやデータ保存先は環境変数で変更できます:

```
set PORT=9000
set MULTI_TERMINALS_DIR=C:\Users\you\mt-data
set MULTI_TERMINALS_SHELL=cmd.exe   :: 既定は powershell.exe
multi-terminals-windows-amd64.exe
```

## 機能

- ターミナルは Windows ネイティブの **ConPTY** で動作（既定シェル: PowerShell）
- 1画面 / 左右2分割 / 上下2分割 / 4分割 + アクティブペイン最大化
- フォルダ・起動コマンド(自動実行/手動)・レイアウトを保存して翌日に再現
- セッションはサーバー側で保持。ブラウザを閉じても継続し、再オープンでスクロールバック復元

## 注意

- **未署名バイナリ**です。初回起動時に SmartScreen の警告が出る場合があります（「詳細情報」→「実行」）。
- このプレリリースは macOS 上でのクロスコンパイル＋コンパイル検証で作成しています。ConPTY の**実機動作は Windows 環境での確認を推奨**します。問題があれば Issue へ。

## チェックサム (SHA-256)

`SHA256SUMS.txt` を参照してください。

```
244c03b0a23baeb3b9a287ce1432cacdb5f4c16f22edee785fba8b020a3a5334  multi-terminals-windows-amd64.exe
5f7ad47ebb4bd75d45418f246ff3ab1cc15552fc1e412a1103d5bb4783423f65  multi-terminals-windows-arm64.exe
```
