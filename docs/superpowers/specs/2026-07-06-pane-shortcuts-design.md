# ペイン操作ショートカット（最大化 / Finder / VS Code / リモート）設計

日付: 2026-07-06
ステータス: 承認済み

## 目的

ペインヘッダのボタンでしか操作できなかった「最大化 / Finder で開く /
VS Code で開く / リモート (GitHub) を開く」をキーボードから実行できるように
する。既存のペイン操作系ショートカット（Ctrl+Shift+矢印）と同じ修飾キー
体系に揃える。

## キー割り当て

すべてアクティブペイン（`activePaneId`、未設定なら先頭ペイン）が対象。

| キー | 動作 |
|---|---|
| Ctrl+Shift+Z | 最大化 / 元に戻す（トグル） |
| Ctrl+Shift+F | Finder で開く |
| Ctrl+Shift+V | VS Code で開く |
| Ctrl+Shift+G | リモート (GitHub) を開く |

検討した代替案: 案B (F/E/R、Linux 端末の Ctrl+Shift+V 貼り付け習慣を回避)、
案C (⌘⇧ ベース)。macOS ではコピペが ⌘C/⌘V で Ctrl+Shift+V の衝突が実質
ないこと、頭文字そのままで覚えやすいことから案A (F/V/G) を採用。

## 挙動の詳細

- ワークスペース未選択、またはペインが 1 つもない場合は何もしない。
- Ctrl+Shift+Z: 最大化中は元に戻す（ヘッダ ⤢ ボタンと同じトグル。
  最大化中は `maximizedPaneId` のペインを対象にする）。
- Ctrl+Shift+G: アクティブペインが git リポジトリでない場合
  （`paneGit[id]?.isRepo` が falsy）は何もしない。🌐 ボタンが非表示に
  なるのと一貫した挙動で、エラーも表示しない。
- ハンドラは既存の `onKey`（window capture リスナー）に追加するため、
  ターミナル (xterm) にフォーカスがあっても効く。Ctrl+Shift+Z/F/V/G は
  端末側に渡らなくなるが、macOS 前提で実害なしと整理。
- タイトル編集などの input にフォーカスがある場合も発火する（既存の
  Ctrl+Shift+矢印と同じ扱い。Ctrl+Shift+文字は入力操作と競合しない）。

## 実装構成

- `frontend/src/lib/shortcuts.js`
  - キーイベント → アクション名を返す pure 関数 `paneShortcutAction(e)`
    を追加。戻り値は `'maximize' | 'finder' | 'vscode' | 'github' | null`。
    判定条件: `ctrlKey && shiftKey && !altKey && !metaKey` かつ
    `e.key` の小文字化が `z/f/v/g`。
  - `SHORTCUT_GROUPS` の「ペイン」グループに 4 項目を追記（ヘルプ
    モーダル ⌘/ に自動反映）。
- `frontend/src/lib/shortcuts.node.test.mjs`
  - `paneShortcutAction` の node テストを追加（各キーのマッピング、
    修飾キー不一致・対象外キーで null）。TDD (Red → Green) で進める。
- `frontend/src/App.svelte`
  - `onKey` 内で `paneShortcutAction(e)` を呼び、`maximize` →
    `toggleMaximize`、`finder`/`vscode`/`github` → `openPaneIn` に
    ディスパッチ。`github` は `isRepo` チェック後に呼ぶ。
- バックエンド変更なし（既存 API `maximizePane` / `restoreLayout` /
  `openPaneIn` をそのまま使用）。

## テスト・検証

- `node --test frontend/src/lib/*.node.test.mjs` がパスすること。
- `npm run build` (frontend) が成功すること。
- 手動確認: 各ショートカットの発火、ヘルプモーダルへの表示、
  非リポジトリペインでの Ctrl+Shift+G 無反応。
