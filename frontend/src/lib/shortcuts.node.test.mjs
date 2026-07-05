import assert from 'node:assert'
import { SHORTCUT_GROUPS } from './shortcuts.js'

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

console.log('shortcuts: OK')
