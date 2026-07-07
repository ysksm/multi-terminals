# multi-terminals

複数のターミナルを1ウィンドウで管理し、フォルダ・起動コマンド・レイアウトを保存して翌日でもすぐに環境を再現できるアプリ。

- 1画面 / 左右2分割 / 上下2分割 / 4分割のプリセットレイアウト + アクティブペイン最大化
- ペインごとに作業ディレクトリと起動コマンド（自動実行/手動）を保存
- ワークスペース一覧に claude code / codex の稼働状況を表示（● 実行中 / ⏸ 許可待ち、件数つき・リアルタイム更新。macOS / Linux）
- 「前回開いた状態」を起動時に復元
- バックエンド: Go（DDD レイヤード + クリーンアーキテクチャ + CQRS）。ターミナルは **Unix=PTY / Windows=ConPTY** にネイティブ対応（[go-pty](https://github.com/aymanbagabas/go-pty)）。
- フロントエンド: Svelte 5 + Vite + xterm.js（ブラウザ）。Wails 版は将来対応予定。

## アーキテクチャ

```
core/domain          … Entities, Value Objects, 集約, Repository/ポート（依存ゼロ・stdlib のみ）
core/application     … CQRS Command/Query ハンドラ, ポート, DTO（stdlib のみ）
core/infrastructure  … JSON 永続化(jsonstore) / ターミナル(go-pty: PTY・ConPTY) / リモート実行(remoteterm: WebSocket + SSH)
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

ペインごとに「リモートホスト」を設定すると、そのペインのターミナルは**別マシン上で実行**され、出力は接続元に返ってストリーム表示されます。リモートホストの書式で **2 つの接続方式**を自動で選択します。

| 入力するリモートホスト | 接続方式 | 相手側に必要なもの |
| --- | --- | --- |
| `192.168.1.10:8080` / `https://host` | **multi-terminals 方式**（WebSocket + Ed25519） | 相手も multi-terminals を起動し、こちらの公開鍵を許可 |
| `ssh://user@host[:port]` | **SSH 方式**（既存 sshd へ接続） | 相手で sshd が動作、こちらの SSH 鍵を authorized_keys に登録 |

```
[マシン A: 接続側]                     [マシン B: 実行側]
ブラウザ ── ws ──> backend A ──┬─ ws ──> backend B ──> PTY   （multi-terminals 方式）
                              └─ ssh ─> sshd ─────> PTY   （SSH 方式）
```

空欄なら従来どおりローカル実行です。どちらの方式でも、ワークスペースを「開く」とリモート側でシェルが起動し、キー入力・リサイズは実行側へ、出力は接続元へ双方向に流れ、スクロールバック復元（ブラウザ再接続時）にも対応します。

### 方式 1: multi-terminals 同士（WebSocket + Ed25519 公開鍵）

待ち受け側・接続側とも同じバイナリを使い、SSH 風のチャレンジ・レスポンスで認証します。

各インスタンスの Ed25519 鍵ペアは**自動生成されません**。「🔑 リモート設定」で**ユーザーが明示的に作成**したときにだけ生成されます（`MULTI_TERMINALS_DIR` 配下の `remote_key` / `remote_key.pub`）。鍵は同画面から**再作成・削除**もできます（再作成すると公開鍵が変わるため、他端末の「許可された鍵」に登録済みの場合は登録し直しが必要）。**待ち受け側の「許可された鍵」リストに載っている公開鍵だけ**が接続でき、リストが空の間は待ち受け自体が無効（403）なので、意図せずシェルが公開されることはありません。秘密情報がネットワークを流れることもありません。

セットアップ手順:

1. **接続側（マシン A）**: サイドバーの「🔑 リモート設定」を開き、「この端末の鍵を作成」で鍵を生成してから、公開鍵（`ed25519:…`）をコピー
2. **待ち受け側（マシン B）**: 同じく「🔑 リモート設定」を開き、「許可された鍵」に A の公開鍵を追加（＝この時点で待ち受けが有効になる）
3. **マシン A**: ペインの追加/編集フォームの「リモートホスト」に B のアドレス（例: `192.168.1.10:8080`、`https://host.example`）を入力

- プロトコル: `GET /api/remote/terminal`（WebSocket）。サーバーが nonce チャレンジ → クライアントが署名（`auth`）→ 制御は JSON テキストフレーム（`start` / `input`(base64) / `resize` / `exit`）、端末出力はバイナリフレーム。実装は `core/infrastructure/remoteterm`。
- 鍵管理 API: `GET /api/remote/identity`（鍵の有無・自分の公開鍵）、`POST /api/remote/identity`（作成。既存なら 409）、`POST /api/remote/identity/regenerate`（再作成）、`DELETE /api/remote/identity`（削除）、`GET/POST/DELETE /api/remote/authorized-keys`（許可リスト）。許可リストはファイル直接編集も可（`remote_authorized_keys`、1行1鍵 `ed25519:<base64> コメント`）。
- **プロトコル（暗号化）の選択**: 平文 `ws://` か TLS `wss://` かは**入力スキームで決まります**。`host:port` / `http://` は `ws://`（平文）、`https://` は `wss://`（TLS）へ自動変換されます。`ws://`（平文）の場合のみ経路が暗号化されないため、平文で使うときは信頼できるネットワーク（VPN / LAN）内に限るか、`https://`（→ `wss://`）で TLS 終端を挟んでください。`wss://` を使えば経路も暗号化されます。

### 方式 2: 既存の SSH サーバへ接続（`ssh://`）

相手に multi-terminals を用意できない・したくない場合は、**普段使っている sshd にそのまま接続**できます。リモートホストに `ssh://user@host[:port]`（ポート既定 22、`user@` 省略時はローカルのログインユーザー）を入力してください。

- **認証は既存の SSH 設定を再利用**します。稼働中の **ssh-agent**（`$SSH_AUTH_SOCK`）→ 次に `~/.ssh` の既定鍵（`id_ed25519` / `id_ecdsa` / `id_rsa`）の順で試します。パスフレーズ付き鍵は agent 経由で使ってください（`ssh-add`）。事前に `ssh-copy-id user@host` で公開鍵を相手の `~/.ssh/authorized_keys` に登録しておきます。
- **ホスト鍵検証**は `~/.ssh/known_hosts` に対して行います。未登録ホストは接続失敗になるので、一度 `ssh user@host` して登録するか、信頼できるネットワークでは `MULTI_TERMINALS_SSH_INSECURE=1` で検証をスキップできます。
- リモート側で PTY（`xterm-256color`）を割り当て、ログインシェルを起動します。実装は `core/infrastructure/remoteterm/ssh.go`。

### macOS のファイアウォール

**方式 1 の待ち受け側（マシン B）**は着信接続を受け付けます。macOS のアプリケーションファイアウォールが有効だと、初回起動時に「"multi-terminals" が着信ネットワーク接続を受け付けることを許可しますか？」と尋ねられます。「拒否」すると接続できません。ブロックされた場合は **システム設定 → ネットワーク → ファイアウォール → オプション** でバイナリを「着信接続を許可」に追加してください。同一 LAN/VPN であれば通常問題ありません。方式 2（`ssh://`）は相手の sshd（既に許可済みのことが多い）へ**こちらから接続する**ため、接続側のファイアウォールは影響しません。

### つながらないときの切り分け

接続元マシンから待ち受け側 B を直接叩いて状態を確認できます（方式 1）:

```sh
# B が起動・到達可能か（許可鍵が空なら 403 が返る＝生きている証拠）
curl -i http://192.168.1.10:8080/api/remote/authorized-keys
# 接続側 A の公開鍵（これを B に登録する）
curl http://localhost:8080/api/remote/identity
```

| 症状 | 主な原因と対処 |
| --- | --- |
| ペインに `HTTP 403 / remote access is disabled` | B に許可鍵が未登録。B の「🔑 リモート設定」に A の公開鍵を追加 |
| 接続拒否・タイムアウト | ポート未指定（`host` だけだと 80 番）・ファイアウォール・別ネットワーク。`host:8080` のようにポートを付ける |
| `unauthorized` | 鍵の登録違い（A の鍵を B に登録する）・コピペ欠け |
| `ssh: host key not in known_hosts` | 一度 `ssh user@host` して登録、または `MULTI_TERMINALS_SSH_INSECURE=1` |
| `ssh: authentication rejected` | 相手の `authorized_keys` に公開鍵未登録（`ssh-copy-id`）・agent 未起動 |

## 環境変数

| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `PORT` | `8080` | バックエンドの待受ポート |
| `MULTI_TERMINALS_DIR` | OS のユーザー設定ディレクトリ配下 `multi-terminals/` | ワークスペース JSON と `app-state.json` の保存先 |
| `MULTI_TERMINALS_SHELL` | （Windows のみ）`powershell.exe` | Windows で使うデフォルトシェル（`cmd.exe` 等に上書き可） |
| `MULTI_TERMINALS_SSH_INSECURE` | （未設定＝検証あり） | `ssh://` 接続で `known_hosts` によるホスト鍵検証をスキップ（`1` 等の非空値で有効）。信頼できる LAN/VPN 限定 |

リモート実行の鍵ファイル（`MULTI_TERMINALS_DIR` 配下。「🔑 リモート設定」から作成/再作成/削除）:

| ファイル | 説明 |
| --- | --- |
| `remote_key` | この端末の Ed25519 秘密鍵（0600、ユーザー操作で作成。自動生成なし） |
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
