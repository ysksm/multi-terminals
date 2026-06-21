# multi-terminals

複数のターミナルを1ウィンドウで管理し、フォルダ・起動コマンド・レイアウトを保存して翌日でもすぐに環境を再現できるアプリ。

- 1画面 / 左右2分割 / 上下2分割 / 4分割のプリセットレイアウト + アクティブペイン最大化
- ペインごとに作業ディレクトリと起動コマンド（自動実行/手動）を保存
- 「前回開いた状態」を起動時に復元
- バックエンド: Go（DDD レイヤード + クリーンアーキテクチャ + CQRS）。ターミナルは **Unix=PTY / Windows=ConPTY** にネイティブ対応（[go-pty](https://github.com/aymanbagabas/go-pty)）。
- フロントエンド: Svelte 5 + Vite + xterm.js（ブラウザ）。Wails 版は将来対応予定。

## アーキテクチャ

```
core/domain          … Entities, Value Objects, 集約, Repository/ポート（依存ゼロ・stdlib のみ）
core/application     … CQRS Command/Query ハンドラ, ポート, DTO（stdlib のみ）
core/infrastructure  … JSON 永続化(jsonstore) / ターミナル(go-pty: PTY・ConPTY)
apps/web             … net/http REST + gorilla/websocket（薄いアダプタ）
frontend             … Svelte + xterm.js（/api を web へプロキシ）
```

外部依存は `infrastructure/terminal`（go-pty）と `apps/web`（gorilla/websocket）に隔離。core は標準ライブラリのみ。

## 必要環境

- Go 1.26+
- Node.js 20+ / npm（フロントエンド開発時）

## 開発起動（2プロセス）

ブラウザ UI は Vite が配信し、`/api`（REST + WebSocket）を Go バックエンドへプロキシします。バックエンドとフロントの2つを起動してください。

### macOS / Linux

```sh
# 端末1: Go バックエンド (:8080)
./scripts/dev.sh web

# 端末2: フロントエンド (:5173)
./scripts/dev.sh frontend
```

ブラウザで **http://localhost:5173** を開く（開発時の :8080 は API 専用です）。

### Windows

`scripts/dev.sh` は bash 用です。PowerShell では生コマンドで起動します。

```powershell
# 端末1: Go バックエンド (:8080)
go run ./apps/web/cmd

# 端末2: フロントエンド (:5173)
cd frontend; npm install; npm run dev
```

ブラウザで **http://localhost:5173** を開く。

## 本番ビルド（単一バイナリ）

フロントエンドをサーバーバイナリに埋め込み、**UI と API を1ポートで配信する単一の成果物**を生成します。

```sh
./scripts/dev.sh build    # frontend をビルドして bin/multi-terminals に組み込む
./scripts/dev.sh start    # bin/multi-terminals を起動（= ./bin/multi-terminals）
```

ブラウザで **http://localhost:8080** を開く（UI が組み込み配信されます。Vite は不要）。
Windows でも同様にバイナリ1つで動きます（PowerShell: `go build -o bin/multi-terminals.exe ./apps/web/cmd` で frontend 組み込みビルドするには先に `cd frontend; npm run build` 後、`apps/web/webui/dist` へ配置）。最も簡単なのは `scripts/dev.sh build`（Git Bash 等）です。

## 環境変数

| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `PORT` | `8080` | バックエンドの待受ポート |
| `MULTI_TERMINALS_DIR` | OS のユーザー設定ディレクトリ配下 `multi-terminals/` | ワークスペース JSON と `app-state.json` の保存先 |
| `MULTI_TERMINALS_SHELL` | （Windows のみ）`powershell.exe` | Windows で使うデフォルトシェル（`cmd.exe` 等に上書き可） |
| `SHELL` | （Unix）`/bin/sh` | Unix のデフォルトシェル |
| `VITE_API_TARGET` | `http://localhost:8080` | Vite プロキシのバックエンド宛先 |

## テスト

```sh
go test ./...          # 全テスト
go test -race ./...    # データ競合検出付き
./scripts/dev.sh check # build + vet + test
```

ターミナル実装テストは実シェルを起動します（Unix=`/bin/sh`、Windows=`cmd.exe`）。
