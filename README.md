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
core/infrastructure  … JSON 永続化(jsonstore) / ターミナル(go-pty: PTY・ConPTY) / リモート実行(remoteterm: WebSocket)
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

## デスクトップ版（Wails）

`apps/wails` は Wails によるネイティブデスクトップ版です。REST/SPA は既存の
mux を在プロセス配信し、端末 I/O は Go↔JS バインディングで動きます（ネット
ワークポートを開きません）。

**UI 開発**は既存の Web 開発フローをそのまま使います（デスクトップ版も同じフロントエンドを共用）:

```sh
./scripts/dev.sh web       # Go バックエンド (:8080)
./scripts/dev.sh frontend  # フロントエンド (:5173)
# ブラウザで http://localhost:5173 を開く
```

> `wails dev` 単体では "not built" ページが表示されます。UI は `apps/web/webui/dist` に
> コンパイル時埋め込みされており、`scripts/build-all.sh`（または後述の手動手順）でのみ
> 生成されるためです。

**デスクトップアプリのビルド・実行**（`build-all.sh` がフロントエンドのビルドと埋め込みも行います）:

```sh
./scripts/build-all.sh wails   # フロントエンドビルド→埋め込み→Wails ビルドを一括実行
./scripts/build-wails.sh       # Wails 専用スクリプト（実行 OS 向けにビルド）
./scripts/build-wails.sh dev   # wails dev で開発起動（埋め込みも自動実行）
```

または `wails build` を直接使う場合は、先に `apps/web/webui/dist` を用意してください:

```sh
cd frontend && npm run build && cd ..
cp -R frontend/dist/. apps/web/webui/dist/
cd apps/wails && wails build
```

ビルド（**クロスコンパイル不可**。Windows 版は Windows 上、macOS 版は macOS 上で）:

```sh
./scripts/build-all.sh all      # web バイナリ + 実行 OS 向け Wails 成果物
./scripts/build-all.sh wails    # Wails のみ
```

Windows / macOS 両方の成果物は GitHub Actions（`.github/workflows/build.yml`）で
ネイティブランナー上から取得できます。

## リモート実行（他の端末で実行する）

ペインごとに「リモートホスト」を設定すると、そのペインのターミナルは**別マシンで動いている multi-terminals 上で実行**され、出力は接続元に返ってストリーム表示されます。待ち受け側・接続側とも同じバイナリです。

```
[マシン A: 接続側]                     [マシン B: 待ち受け側]
ブラウザ ── ws ──> backend A ── ws ──> backend B ──> PTY（B のローカルで実行）
                     └─ /api/remote/terminal（Bearer トークン認証）─┘
```

### 認証（Ed25519 公開鍵・自動生成）

各インスタンスは**初回起動時に Ed25519 鍵ペアを自動生成**します（`MULTI_TERMINALS_DIR` 配下の `remote_key` / `remote_key.pub`）。接続は SSH 風のチャレンジ・レスポンスで認証され、**待ち受け側の「許可された鍵」リストに載っている公開鍵だけ**が接続できます。リストが空の間は待ち受け自体が無効（403）なので、意図せずシェルが公開されることはありません。秘密情報がネットワークを流れることもありません。

### セットアップ手順

1. **接続側（マシン A）**: サイドバーの「🔑 リモート設定」を開き、「この端末の公開鍵」（`ed25519:…`）をコピー
2. **待ち受け側（マシン B）**: 同じく「🔑 リモート設定」を開き、「許可された鍵」に A の公開鍵を追加（＝この時点で待ち受けが有効になる）
3. **マシン A**: ペインの追加/編集フォームの「リモートホスト」に B のアドレス（例: `192.168.1.10:8080`、`https://host.example`）を入力。空欄なら従来どおりローカル実行

ワークスペースを「開く」と、リモートホスト付きペインはマシン B 上でシェルを起動し、キー入力・リサイズは B へ、出力は A へ双方向に流れます。リモートペインもスクロールバック復元（ブラウザ再接続時）に対応します。

- プロトコル: `GET /api/remote/terminal`（WebSocket）。サーバーが nonce チャレンジ → クライアントが署名（`auth`）→ 制御は JSON テキストフレーム（`start` / `input`(base64) / `resize` / `exit`）、端末出力はバイナリフレーム。実装は `core/infrastructure/remoteterm`。
- 鍵管理 API: `GET /api/remote/identity`（自分の公開鍵）、`GET/POST/DELETE /api/remote/authorized-keys`（許可リスト）。ファイル直接編集も可（`remote_authorized_keys`、1行1鍵 `ed25519:<base64> コメント`）。
- セキュリティ: リモート受付はシェル実行そのものを公開する機能です。鍵認証によりなりすまし・盗聴による資格情報漏えいは防げますが、経路の暗号化はしないため、信頼できるネットワーク（VPN / LAN）内で使うか、TLS 終端（`https://` → `wss://` 自動変換に対応）を挟んでください。

## 環境変数

| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `PORT` | `8080` | バックエンドの待受ポート |
| `MULTI_TERMINALS_DIR` | OS のユーザー設定ディレクトリ配下 `multi-terminals/` | ワークスペース JSON と `app-state.json` の保存先 |
| `MULTI_TERMINALS_SHELL` | （Windows のみ）`powershell.exe` | Windows で使うデフォルトシェル（`cmd.exe` 等に上書き可） |

リモート実行の鍵ファイル（`MULTI_TERMINALS_DIR` 配下、自動生成・管理）:

| ファイル | 説明 |
| --- | --- |
| `remote_key` | この端末の Ed25519 秘密鍵（0600、初回起動時に自動生成） |
| `remote_key.pub` | 対応する公開鍵（他の端末に登録する値） |
| `remote_authorized_keys` | この端末での実行を許可する公開鍵リスト（空 = 待ち受け無効） |
| `SHELL` | （Unix）`/bin/sh` | Unix のデフォルトシェル |
| `VITE_API_TARGET` | `http://localhost:8080` | Vite プロキシのバックエンド宛先 |

## テスト

```sh
go test ./...          # 全テスト
go test -race ./...    # データ競合検出付き
./scripts/dev.sh check # build + vet + test
```

ターミナル実装テストは実シェルを起動します（Unix=`/bin/sh`、Windows=`cmd.exe`）。
