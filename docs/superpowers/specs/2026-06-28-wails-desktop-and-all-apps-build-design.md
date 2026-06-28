# Wails デスクトップ版 + 全アプリ一括ビルド 設計書

- 日付: 2026-06-28
- 対象ブランチ: feat/build-and-serve（または派生ブランチ）
- ステータス: 承認済み（実装計画へ）

## 目的

1. Go 製デスクトップアプリを **Wails** で追加し、**Windows / macOS 両対応**でビルドできるようにする。
2. **全アプリ（web サーバ版 + Wails デスクトップ版）を一括ビルドするシェルスクリプト**を用意する。

## 前提・制約（確定事実）

- フロントエンド（Svelte 5 + Vite + xterm.js）は **同一オリジン前提**:
  - REST: `frontend/src/lib/api.js` が相対パス `fetch('/api/...')`。
  - 端末 I/O: `frontend/src/lib/Terminal.svelte` が `ws(s)://${location.host}/api/panes/{id}/io` に WebSocket 接続。
- 既存の web 版は Go の HTTP サーバ（`apps/web` の `web.NewMux`）+ 埋め込み UI（`apps/web/webui`、`//go:embed all:dist`）で、UI と API を 1 ポートで配信する。
- 端末 I/O の実体:
  - `deps.Registry.Get(paneID)` → `hub`
  - `hub.Subscribe()` → `(snapshot []byte, sub)`、`sub.C()` が出力チャンク、`sub.Done()` が終了通知
  - `hub.Unsubscribe(sub)`
  - 入力: `deps.Write.Handle(ctx, command.WriteToPaneCommand{PaneID, Data})`
  - リサイズ: `deps.Resize.Handle(ctx, command.ResizePaneCommand{PaneID, Cols, Rows})`
- **Wails はクロスコンパイル不可**: Windows 版は Windows 上（WebView2 + CGO）、macOS 版は macOS 上（WebKit）でのみビルド可能。plain `go build` のクロスビルドとは事情が異なる。

## 決定事項（ブレインストーミング結果）

1. **Wails の中身 = ネイティブバインディング**（在プロセス。localhost TCP ポートを開かない）。
2. **転送層は両対応・自動判定**: Wails 環境（`window.runtime`/`window.go` 検出）ならバインディング、それ以外は従来の WS/fetch。web 版はそのまま動作。
3. **両 OS 対応 = ビルドスクリプト + CI の両方**: ローカルスクリプトは実行 OS 向けをビルドし他 OS は警告スキップ。GitHub Actions で windows/macos ランナー上から両方を自動ビルドして成果物を取得。

## アーキテクチャ

### コンポーネント構成

```
apps/
  web/            既存: HTTP サーバ版（変更なし、唯一フロントの同一オリジン契約を共有）
    webui/        既存: //go:embed all:dist（UI 埋め込み）
  wails/          新規: Wails デスクトップ版
    main.go       wails.Run（AssetServer.Handler に既存 mux、Bind に App）
    app.go        App 構造体（端末 I/O のネイティブバインディング）
    wails.json    frontend:dir = ../../frontend
frontend/
  src/lib/
    termTransport.js  新規: 端末転送の抽象（desktop=bindings / browser=WS）
    Terminal.svelte   変更: 直接 WS 生成をやめ termTransport 経由へ
    api.js            無改修（相対 fetch がそのまま動く）
scripts/
  build-all.sh    新規: 全アプリ一括ビルドのオーケストレータ
  build-mac.sh    既存: web の darwin 向けビルド（build-all から流用/呼び出し）
.github/workflows/
  build.yml       新規: macos/windows マトリクスで両 OS を自動ビルド
```

### 1. Wails アプリ（`apps/wails/`）

**UI 埋め込みの再利用**: `apps/wails` から `apps/web/webui` と `apps/web` をインポートする。`wails build` 時に webui の `//go:embed all:dist` が推移的に効くため、Wails 専用の dist コピーや別 embed は不要。ビルド前に `apps/web/webui/dist` を用意しておくだけでよい。

- **`apps/wails/main.go`**:
  - `deps, _ := web.BuildDeps(baseDir)`（baseDir は `MULTI_TERMINALS_DIR` か既定）。
  - `mux := web.NewMux(deps); mux.Handle("/", webui.Handler())`（main の web と同じ組み立て）。
  - `app := NewApp(deps)`。
  - `wails.Run(&options.App{ AssetServer: &assetserver.Options{Handler: mux}, OnStartup: app.startup, Bind: []interface{}{app}, ... })`。
  - 終了時に `deps.Registry.CloseAll()` を呼ぶ（OnShutdown）。
  - 結果: REST と SPA は AssetServer.Handler 経由で **在プロセス配信**（ネットワークポート不要）。端末のみバインディング。

- **`apps/wails/app.go`**（`App` 構造体）:
  - フィールド: `ctx context.Context`, `deps web.Deps`, 購読管理マップ（paneID→解除関数）, `sync.Mutex`。
  - `startup(ctx)`: ctx を保持。
  - `PaneSubscribe(paneID string) error`: `deps.Registry.Get(paneID)` → 無ければエラー。`hub.Subscribe()` のスナップショットを `runtime.EventsEmit(ctx, "pane:"+paneID, base64(snapshot))`。goroutine で `sub.C()` を読み `EventsEmit`、`sub.Done()` で終了イベント `pane:"+paneID+":done"` を送り `Unsubscribe`。多重購読は解除関数で管理。
  - `PaneUnsubscribe(paneID string)`: 保持した解除関数を呼ぶ。
  - `PaneWrite(paneID string, data string) error`: `deps.Write.Handle(ctx, command.WriteToPaneCommand{PaneID: paneID, Data: []byte(data)})`。
  - `PaneResize(paneID string, cols uint16, rows uint16) error`: `deps.Resize.Handle(ctx, command.ResizePaneCommand{...})`。
  - バイナリ安全性のため、チャンク/入力は base64 で JS と受け渡す（端末出力は任意バイト列を含むため）。

- **`apps/wails/wails.json`**:
  - `frontend:dir = ../../frontend`、`frontend:install = npm install`、`frontend:build = npm run build`。
  - `wailsjsdir` は使わず、フロントは実行時 `window.go.main.App.*` を直接参照（型生成に依存しない疎結合）。

### 2. フロントエンド転送層

- **`frontend/src/lib/termTransport.js`**（新規）:
  - `isDesktop()`: `typeof window !== 'undefined' && !!(window.runtime && window.go)`。
  - `connectPane(paneId, { onData, onClose })` を返す共通 API。戻り値は `{ send(data), resize(cols, rows), close() }`。
    - desktop: `window.runtime.EventsOn("pane:"+paneId, b64 => onData(base64ToBytes(b64)))`、`EventsOn("pane:"+paneId+":done", onClose)`、`window.go.main.App.PaneSubscribe(paneId)`。`send` は `PaneWrite`、`resize` は `PaneResize`、`close` は `EventsOff` + `PaneUnsubscribe`。
    - browser: 現 `Terminal.svelte` の WS ロジックをそのまま移設（`ws(s)://${location.host}/api/panes/{id}/io`、バイナリフレーム = 出力、`{type:'input'|'resize'}` = 入力）。
- **`frontend/src/lib/Terminal.svelte`**（変更）:
  - 直接 `new WebSocket(...)` をやめ、`connectPane()` を使う。xterm への書き込み/入力/リサイズはハンドラ経由。
  - web 版の挙動・見た目は不変（同じ WS パスを使う）。

### 3. ビルドスクリプト（`scripts/build-all.sh`）

実行 OS を判定し、以下を順に実施:

1. **共有フロントビルド**: `frontend` を `npm ci/install` → `npm run build` → `apps/web/webui/dist` へコピー（既存 build スクリプトと同じ埋め込み手順）。
2. **web バイナリ（クロスビルド可能）**: `CGO_ENABLED=0` で
   - darwin/arm64, darwin/amd64, lipo universal（macOS 実行時のみ universal）
   - windows/amd64, windows/arm64
   を `release/` に出力。
3. **Wails（実行 OS 向けのみ）**:
   - `wails` CLI 未導入 → 警告して案内（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）しスキップ。
   - macOS: `wails build -platform darwin/universal` → `.app` を `release/` へ。
   - Windows: `wails build -platform windows/amd64`（および arm64）→ `.exe` を `release/` へ。
   - 非対象 OS 分は「この OS ではビルド不可。CI もしくは該当 OS で実行してください」と明示。
4. **チェックサム & サマリ**: `release/SHA256SUMS.txt` を生成し、生成物一覧を表示。

引数で対象を絞れる: `build-all.sh [all|web|wails]`（既定 all）。

### 4. CI（`.github/workflows/build.yml`）

- トリガ: `workflow_dispatch` と タグ push（`v*`）。
- マトリクス: `os: [macos-latest, windows-latest]`。
- 各ジョブ: Go セットアップ、Node セットアップ、`go install wails CLI`、`frontend` ビルド、`apps/web/webui/dist` 埋め込み、
  - macos: web darwin バイナリ + `wails build -platform darwin/universal`
  - windows: web windows バイナリ + `wails build -platform windows/amd64`
- `actions/upload-artifact` で `release/` を OS 別にアップロード。
- ランナー前提: macOS ランナーは Xcode/WebKit 同梱、Windows ランナーは WebView2 同梱のため追加インストール不要。

## データフロー

### 端末 I/O（desktop）

```
xterm(JS) --input(b64)--> App.PaneWrite --> deps.Write.Handle --> PTY
PTY --output--> hub.Subscribe.sub.C() --> runtime.EventsEmit("pane:<id>") --> xterm(JS)
resize: xterm --> App.PaneResize --> deps.Resize.Handle
```

### 端末 I/O（browser, 不変）

```
xterm(JS) <--WebSocket /api/panes/<id>/io--> ws.go(handlePaneIO) <--> hub / Write / Resize
```

### REST（両環境共通）

```
api.js fetch('/api/...') --> [browser] HTTP server / [desktop] Wails AssetServer.Handler(mux)
```

## エラーハンドリング

- `PaneSubscribe`: セッション未存在は error を返し、JS 側は接続失敗として扱う（web の 404 相当）。
- 二重購読/解除は解除関数の有無で冪等化。
- Wails 終了時に `deps.Registry.CloseAll()` で PTY 子プロセスを確実に終了。
- ビルドスクリプト: `set -euo pipefail`。wails 未導入・非対象 OS は致命的エラーにせず警告スキップ（web ビルドは続行）。

## テスト戦略

- **Go**: `apps/wails/app.go` の端末バインディング（PaneWrite/Resize/Subscribe）はフェイクの Registry/ハンドラに対するユニットテストで検証。`wails.Run` 自体は GUI 依存のため直接テストしない。
- **frontend**: `termTransport.js` の分岐（isDesktop 判定、browser 経路の URL 組み立て）を軽量に確認。Terminal.svelte は手動スモーク。
- **既存テスト**: web 版の挙動は不変。`scripts/dev.sh check`（build + vet + test）が緑であること。
- **スモーク**: build-all で生成した web バイナリ起動で `GET /`=200, `/api/sessions`=200。Wails は実行 OS で起動確認（GUI 表示・端末入出力）。

## スコープ外（YAGNI）

- コード署名 / 公証（unsigned。NOTES に SmartScreen/Gatekeeper 警告を注記）。
- 自動アップデータ。
- Wails の実験的クロスコンパイル（NSIS/Docker 経由）。CI で各ネイティブランナーを使うため不要。
- Linux 対応（要望なし）。

## 成果物一覧

- `apps/wails/{main.go,app.go,wails.json}`
- `apps/wails/app_test.go`
- `frontend/src/lib/termTransport.js`、`frontend/src/lib/Terminal.svelte`（更新）
- `scripts/build-all.sh`
- `.github/workflows/build.yml`
- `README.md` / `release/NOTES.md` の追記（デスクトップ版の起動・注意）
