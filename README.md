# multi-terminals

複数のターミナルを1ウィンドウで管理し、フォルダ・起動コマンド・レイアウトを保存して翌日でもすぐに環境を再現できるアプリ。

- 1画面 / 左右2分割 / 上下2分割 / 4分割のプリセットレイアウト + アクティブペイン最大化
- ペインごとに作業ディレクトリと起動コマンド（自動実行/手動）を保存
- ワークスペース一覧に claude code / codex の稼働状況を表示（⏸ 応答待ち / ● 処理中 / ✓ 完了(未確認) / ○ 入力待ち、件数つき・リアルタイム更新。macOS / Linux）。判定はサーバ側の仮想端末で描画した画面と OSC タイトルに対するルール照合（[herdr](https://github.com/herdrdev/herdr) の検知マニフェストを移植）
- **アクティブなターミナル集約ビュー**: 起動中のターミナルだけを全ワークスペース横断で 1 画面に集める（`Ctrl+Shift+A`。「エージェント稼働中のみ」で claude / codex が動いているものに絞り込み）
- **タスク管理（Jira 連携 / ローカルタスク）**: Jira の issue を番号で取り込むか、Jira を使わずに手動で作成し、環境テンプレートから作業フォルダ・ブランチ・リポジトリ（git clone / ベースクローンからコピー）・ワークスペースを一括セットアップ。進捗はステップ実行ログで確認でき、失敗したステップから再実行 / スキップできる（Jira は読み取り専用）
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

## 初回セットアップ

クローン直後は次を一度実行すると、Go モジュール・フロントエンド依存・wails CLI をまとめて導入します。

```sh
./scripts/init.sh            # 全部入り
./scripts/init.sh --no-wails # デスクトップ版を使わない場合
```

実行しなくても、各ビルドスクリプト（`dev.sh build` / `build-all.sh` / `build-wails.sh`）が
`npm install` や wails CLI の導入を初回に自動で行います。

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

## アクティブなターミナルを集めて見る

ワークスペースをまたいで**今動いているターミナルだけ**を扱う機能です。

- サイドバーの「ワークスペース」の下に **⚡ アクティブ** 一覧が常時表示されます。起動中のペインを全ワークスペース横断で並べ、クリックするとそのペインのワークスペースへ移動します。各セクションは見出しクリックで折りたためます。
- 一覧下の「⤢ 集約表示」、または `Ctrl+Shift+A` で、起動中のターミナルだけを 1 画面に並べる**集約ビュー**に切り替えます。

- 既定では**起動中（セッションが生きている）ペイン**を全ワークスペースから集めます。ワークスペースを切り替えなくても、そのまま入力・操作できます。
- 「エージェントのみ」（集約ビューでは「エージェント稼働中のみ」）にチェックすると、**claude / codex が動いているペインだけ**に絞り込みます（macOS / Linux。Windows はプロセス走査ができないため常に 0 件）。
- 並び順は **⏸ 応答待ち → ● 処理中 → ✓ 完了(未確認) → ○ 入力待ち → その他**。ユーザーの応答待ちで止まっているターミナルが常に先頭に来ます。「完了(未確認)」は見ていないペインで処理が終わったもので、そのペインをアクティブにすると消えます。
- 各セルの `↗` を押すと、そのペインを従来のワークスペース表示で開きます（該当ワークスペースへ移動）。
- 件数に応じて格子が自動で組まれます（最大 4 列、それ以上は縦スクロール）。

設計メモ: `docs/superpowers/specs/2026-08-15-active-terminals-view-design.md`

## タスク管理（Jira 連携 + 環境セットアップ）

Jira の issue を「タスク」として取り込み、テンプレートに沿って作業環境を自動で用意します。設計は `docs/superpowers/specs/2026-09-10-task-management-design.md`、画面モックは `docs/mockups/2026-09-10-task-management-ui.html`。

### 使い方

1. **Jira 接続**: 右上「⚙ 設定 → Jira 接続」で種類（Jira Cloud: メール + API トークン / Jira Server・Data Center: Personal Access Token）とサイト URL を登録し、「接続テスト」で確認。トークンは `MULTI_TERMINALS_DIR/jira_token` に権限 0600 で保存され、設定 JSON には書かれません
2. **環境テンプレート**: 「⚙ 設定 → 環境テンプレート」で、作業フォルダ（例 `~/work/{KEY}`）・ブランチ名（例 `feature/{KEY}-{slug}`）・レイアウト・リポジトリ（取得方法とセットアップコマンド）・ペイン割当（どのリポジトリでどのコマンドを起動するか）を定義。`{KEY}` は Jira 番号、`{slug}` は概要から作る ASCII の短い識別子（日本語だけの概要では省略）
3. **ベースクローン**（任意）: 「⚙ 設定 → ベースクローン」に clone を登録しておくと、テンプレートで「ベースクローンからコピー」を選べます。`git clone` より速く、`node_modules` を含めてコピーすることもできます（「セットアップ設定」で切替）。セットアップ直前にベースを自動で fetch / pull します
4. **取り込み / 作成**: サイドバー「すべてのタスク」→「＋ タスクを追加」。
   - **Jira から取り込む**: 番号をカンマ区切りで入力（URL 貼り付け可）→「取得」でプレビュー → テンプレートを選んで「取り込んでセットアップ」
   - **手動で作成（ローカルタスク）**: Jira に無い作業（調査・環境整備・個人的な TODO など）を概要・説明・優先度・ラベル・参考リンク・ステータス付きで登録。番号は `T-001` のように自動採番（接頭辞と桁数は「⚙ 設定 → ローカルタスク」で変更）。Jira 接続が未設定でも使えます
5. **セットアップ**: 作業フォルダ作成 → ベース更新 → リポジトリ取得 → fetch とブランチ作成 → セットアップコマンド → ワークスペース作成 の順にステップ実行し、ログを表示します。失敗したステップで止まり「このステップから再実行」「スキップして続行」「中止」を選べます。「バックグラウンドで続行」で一覧に戻っても実行は続きます
6. **作業**: タスク一覧の「開く」または詳細の「ワークスペースを開く」で該当ワークスペースへ。ワークスペース上部にタスク帯（番号・概要・Jira ステータス・ブランチ）が出ます

### ローカル状態

| 状態 | 意味 |
| --- | --- |
| 未準備 | 取り込んだだけ。作業フォルダなし |
| 準備中 | セットアップ実行中 |
| 環境あり | 作業フォルダとワークスペースが準備済み |
| 作業中 | 環境ありで、ワークスペースのターミナルが起動している（導出。保存しない） |
| 完了 | 手動で「完了にする」を押したもの（Jira の状態からは自動で変わらない） |

- サイドバーの「タスク」欄には作業中 / 準備中 / 環境ありだけを最大 5 件表示し、全件はタスク一覧（フィルタ・並び替え・ページング）で扱います
- 「⟳ Jira を同期」で全タスクを再取得します（手動のみ）。接続設定に自動取り込み JQL があれば、該当 issue を未準備で追加します。ローカルタスクは同期の対象外です
- **ローカルタスク**は一覧・詳細で概要・説明・優先度・ラベル・リンク・ステータスを直接編集できます（自動保存）。ステータスは「⚙ 設定 → ローカルタスク」で名前と分類（未着手 / 進行中 / 完了）を定義します。Jira タスクの同じ項目は読み取り専用です
- あとから Jira に起票した場合は、詳細の「Jira に紐付ける」に番号を入れると Jira タスクに切り替わります。番号は Jira のものに変わりますが、作業フォルダ・ブランチ・ワークスペースはそのままです
- 作業フォルダの削除は完了タスクの「作業フォルダを削除」またはタスク詳細から明示的に行ったときだけです。ワークスペースを削除してもフォルダは残ります
- Jira への書き込み（ステータス遷移・コメント）は行いません

### 保存先

`MULTI_TERMINALS_DIR` 配下: `tasks/`、`templates/`、`base_clones/`、`setup_runs/`（タスクごとに直近 1 件）、`task_settings.json`、`jira.json`、`jira_token`（0600）、`base/<name>/`（ベースクローン本体の既定パス）。

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

1. **接続側（マシン A）**: 右上の「⚙ 設定」メニューから「🔑 リモート設定」を開き、「この端末の鍵を作成」で鍵を生成してから、公開鍵（`ed25519:…`）をコピー
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
| `HOST` | （空 = 全インターフェース） | バックエンドの bind アドレス（例: `127.0.0.1` でローカル限定） |
| `PORT` | `8080` | バックエンドの待受ポート |
| `MULTI_TERMINALS_DIR` | OS のユーザー設定ディレクトリ配下 `multi-terminals/` | ワークスペース JSON と `app-state.json` の保存先 |
| `MULTI_TERMINALS_SHELL` | （Windows のみ）`powershell.exe` | Windows で使うデフォルトシェル（`cmd.exe` 等に上書き可） |
| `MULTI_TERMINALS_SSH_INSECURE` | （未設定＝検証あり） | `ssh://` 接続で `known_hosts` によるホスト鍵検証をスキップ（`1` 等の非空値で有効）。信頼できる LAN/VPN 限定 |

`HOST` / `PORT` は、サーバ起動時のカレントディレクトリに置いた **`.env` ファイル**でも設定できます（優先順位: 環境変数 > `.env` > 既定値）。

```sh
# .env の例
HOST=127.0.0.1
PORT=9000
```

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
