import assert from 'node:assert'
import { SHORTCUT_GROUPS, paneShortcutAction } from './shortcuts.js'

assert.ok(Array.isArray(SHORTCUT_GROUPS) && SHORTCUT_GROUPS.length > 0, 'groups: non-empty array')

for (const g of SHORTCUT_GROUPS) {
  assert.ok(typeof g.label === 'string' && g.label.trim(), `group label non-empty: ${JSON.stringify(g)}`)
  assert.ok(Array.isArray(g.items) && g.items.length > 0, `group "${g.label}": non-empty items`)
  for (const item of g.items) {
    assert.ok(
      Array.isArray(item.keys) && item.keys.length > 0 && item.keys.every((k) => typeof k === 'string' && k.trim()),
      `item keys non-empty tokens: ${JSON.stringify(item)}`
    )
    assert.ok(typeof item.desc === 'string' && item.desc.trim(), `item desc non-empty: ${JSON.stringify(item)}`)
  }
}

// グループ名の重複なし
const labels = SHORTCUT_GROUPS.map((g) => g.label)
assert.equal(new Set(labels).size, labels.length, 'group labels unique')

// paneShortcutAction: キーイベント → ペイン操作アクション名
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
assert.equal(paneShortcutAction(ev('B')), 'gitmenu', 'Ctrl+Shift+B → gitmenu')
assert.equal(paneShortcutAction(ev('X')), null, '対象外キーは null')
assert.equal(paneShortcutAction(ev('Z', { ctrlKey: false })), null, 'Ctrl なしは null')
assert.equal(paneShortcutAction(ev('Z', { shiftKey: false })), null, 'Shift なしは null')
assert.equal(paneShortcutAction(ev('Z', { altKey: true })), null, 'Alt 付きは null')
assert.equal(paneShortcutAction(ev('Z', { metaKey: true })), null, 'Meta 付きは null')

// ヘルプ一覧: ペイングループに 5 ショートカットが載っている
const paneGroup = SHORTCUT_GROUPS.find((g) => g.label === 'ペイン')
assert.ok(paneGroup, 'ペイングループが存在する')
for (const kw of ['最大化', 'Finder', 'VS Code', 'リモート', 'git メニュー']) {
  assert.ok(
    paneGroup.items.some((i) => i.desc.includes(kw)),
    `ペイングループに「${kw}」の項目がある`
  )
}

console.log('shortcuts: OK')
