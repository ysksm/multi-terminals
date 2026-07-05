# ペイン操作ショートカット Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** アクティブペインの「最大化 / Finder / VS Code / リモート(GitHub)」をキーボード（Ctrl+Shift+Z/F/V/G）から実行できるようにする。

**Architecture:** キーイベント→アクション名の判定を pure 関数 `paneShortcutAction` として `frontend/src/lib/shortcuts.js` に置き、node スクリプトテストで検証。`App.svelte` の既存 `onKey`（window capture）はその結果を既存の `toggleMaximize` / `openPaneIn` にディスパッチするだけ。バックエンド変更なし。

**Tech Stack:** Svelte 5 (runes), plain-script node tests (`node <file>.node.test.mjs`), Vite build.

**Spec:** `docs/superpowers/specs/2026-07-06-pane-shortcuts-design.md`

## Global Constraints

- キー割り当て: Ctrl+Shift+Z=最大化トグル, F=Finder, V=VS Code, G=リモート(GitHub)。修飾キーは `ctrlKey && shiftKey && !altKey && !metaKey` 厳密一致。
- 対象は `activePaneId`（未設定なら先頭ペイン）。ワークスペース未選択・ペイン 0 個なら no-op。
- Ctrl+Shift+G はアクティブペインが git リポジトリでない（`paneGit[id]?.isRepo` falsy）とき no-op、エラー表示なし。
- テストは既存の plain-script assert 形式（`node:test` ランナー不使用）に合わせる。
- コメント・UI 文言は既存にならい日本語。

---

### Task 1: `paneShortcutAction` pure 関数 + ヘルプ一覧更新

**Files:**
- Modify: `frontend/src/lib/shortcuts.js`
- Test: `frontend/src/lib/shortcuts.node.test.mjs`

**Interfaces:**
- Produces: `paneShortcutAction(e) => 'maximize' | 'finder' | 'vscode' | 'github' | null`。`e` は `{ctrlKey, shiftKey, altKey, metaKey, key}` を持つ KeyboardEvent 互換オブジェクト。Task 2 が `App.svelte` から import する。

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/lib/shortcuts.node.test.mjs` の末尾（`console.log('shortcuts: OK')` の前）に追加:

```js
// paneShortcutAction: キーイベント → ペイン操作アクション名
import { paneShortcutAction } from './shortcuts.js'

const ev = (key, mods = {}) => ({
  ctrlKey: true,
  shiftKey: true,
  altKey: false,
  metaKey: false,
  key,
  ...mods,
})

assert.equal(paneShortcutAction(ev('Z')), 'maximize', 'Ctrl+Shift+Z → maximize')
assert.equal(paneShortcutAction(ev('z')), 'maximize', '小文字 key でも maximize')
assert.equal(paneShortcutAction(ev('F')), 'finder', 'Ctrl+Shift+F → finder')
assert.equal(paneShortcutAction(ev('V')), 'vscode', 'Ctrl+Shift+V → vscode')
assert.equal(paneShortcutAction(ev('G')), 'github', 'Ctrl+Shift+G → github')
assert.equal(paneShortcutAction(ev('X')), null, '対象外キーは null')
assert.equal(paneShortcutAction(ev('Z', { ctrlKey: false })), null, 'Ctrl なしは null')
assert.equal(paneShortcutAction(ev('Z', { shiftKey: false })), null, 'Shift なしは null')
assert.equal(paneShortcutAction(ev('Z', { altKey: true })), null, 'Alt 付きは null')
assert.equal(paneShortcutAction(ev('Z', { metaKey: true })), null, 'Meta 付きは null')

// ヘルプ一覧: ペイングループに 4 ショートカットが載っている
const paneGroup = SHORTCUT_GROUPS.find((g) => g.label === 'ペイン')
assert.ok(paneGroup, 'ペイングループが存在する')
for (const kw of ['最大化', 'Finder', 'VS Code', 'リモート']) {
  assert.ok(
    paneGroup.items.some((i) => i.desc.includes(kw)),
    `ペイングループに「${kw}」の項目がある`
  )
}
```

注: import はファイル先頭の既存 import 行の隣にまとめてもよい（ESM の hoisting でどちらでも動くが、先頭推奨）。

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd frontend && node src/lib/shortcuts.node.test.mjs`
Expected: FAIL — `SyntaxError: The requested module './shortcuts.js' does not provide an export named 'paneShortcutAction'`

- [ ] **Step 3: 最小実装**

`frontend/src/lib/shortcuts.js` — 「ペイン」グループの items を差し替え、関数を追加:

```js
  {
    label: 'ペイン',
    items: [
      { keys: ['Ctrl', 'Shift', '← → ↑ ↓'], desc: '隣のペインへフォーカス移動' },
      { keys: ['Ctrl', 'Shift', 'Z'], desc: 'アクティブペインを最大化 / 元に戻す' },
      { keys: ['Ctrl', 'Shift', 'F'], desc: 'アクティブペインを Finder で開く' },
      { keys: ['Ctrl', 'Shift', 'V'], desc: 'アクティブペインを VS Code で開く' },
      { keys: ['Ctrl', 'Shift', 'G'], desc: 'リモート (GitHub) を開く（リポジトリのみ）' },
    ],
  },
```

ファイル末尾に追加:

```js
/** Ctrl+Shift+英字のペイン操作アクション名。対象外のイベントは null。 */
const PANE_ACTIONS = { z: 'maximize', f: 'finder', v: 'vscode', g: 'github' }

/**
 * キーイベントをペイン操作アクションへ対応付ける pure 関数。
 * @param {{ctrlKey: boolean, shiftKey: boolean, altKey: boolean, metaKey: boolean, key: string}} e
 * @returns {'maximize'|'finder'|'vscode'|'github'|null}
 */
export function paneShortcutAction(e) {
  if (!e.ctrlKey || !e.shiftKey || e.altKey || e.metaKey) return null
  return PANE_ACTIONS[e.key?.toLowerCase()] ?? null
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `cd frontend && node src/lib/shortcuts.node.test.mjs`
Expected: `shortcuts: OK`（assert 失敗なし・exit 0）

他の node テストも回す: `node src/lib/workspaceNav.node.test.mjs`
Expected: `workspaceNav: OK`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/shortcuts.js frontend/src/lib/shortcuts.node.test.mjs
git commit -m "feat(ui): map Ctrl+Shift+Z/F/V/G to pane actions in shortcuts module"
```

---

### Task 2: `App.svelte` の `onKey` にディスパッチを追加

**Files:**
- Modify: `frontend/src/App.svelte`（import 行と `onKey` 関数。現行 233〜279 行付近）

**Interfaces:**
- Consumes: Task 1 の `paneShortcutAction(e)`。既存の `toggleMaximize(paneId)`, `openPaneIn(paneId, target)`, `maximized`, `activePaneId`, `paneGit`。

- [ ] **Step 1: import を更新**

```js
import { SHORTCUT_GROUPS, paneShortcutAction } from './lib/shortcuts.js'
```

- [ ] **Step 2: `onKey` にディスパッチを追加**

既存の Ctrl+Alt+↑/↓ ブロックの直後、`if (!(e.ctrlKey && e.shiftKey) || ...)` の行の**前**に挿入:

```js
    // Ctrl+Shift+Z/F/V/G: アクティブペインの最大化 / Finder / VS Code / リモート
    const paneAction = paneShortcutAction(e)
    if (paneAction) {
      const pane = current?.panes?.find((p) => p.id === activePaneId) || current?.panes?.[0]
      if (!pane) return
      // リポジトリでないペインでは 🌐 ボタン非表示と同様に無視する
      if (paneAction === 'github' && !paneGit[pane.id]?.isRepo) return
      e.preventDefault()
      e.stopPropagation()
      if (paneAction === 'maximize') toggleMaximize(maximized || pane.id)
      else openPaneIn(pane.id, paneAction)
      return
    }
```

- [ ] **Step 3: ビルドで検証**

Run: `cd frontend && npm run build`
Expected: `✓ built in ...`（エラーなし）

Run: `node src/lib/shortcuts.node.test.mjs && node src/lib/workspaceNav.node.test.mjs`
Expected: 両方 OK

- [ ] **Step 4: 手動確認（アプリ起動）**

`scripts/` の起動方法（例: `go run ./apps/web/cmd`）でアプリを起動し、ブラウザで:
- Ctrl+Shift+Z でアクティブペインが最大化 → もう一度で復帰
- Ctrl+Shift+F で Finder、Ctrl+Shift+V で VS Code が開く
- リポジトリのペインで Ctrl+Shift+G でリモートが開く。非リポジトリのペインでは何も起きない
- ⌘/ のヘルプに 4 項目が表示される

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.svelte
git commit -m "feat(ui): keyboard shortcuts for pane maximize and open-in actions"
```
