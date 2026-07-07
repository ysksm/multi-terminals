import assert from 'node:assert'
import { menuKeyAction, filterBranches } from './gitMenu.js'

const st = (branchCount, selectedIndex) => ({ branchCount, selectedIndex })

// 移動: 端でクランプ
assert.deepEqual(menuKeyAction('ArrowDown', st(3, 0)), { type: 'move', index: 1 }, '↓ で次へ')
assert.deepEqual(menuKeyAction('ArrowDown', st(3, 2)), { type: 'move', index: 2 }, '末尾で止まる')
assert.deepEqual(menuKeyAction('ArrowUp', st(3, 2)), { type: 'move', index: 1 }, '↑ で前へ')
assert.deepEqual(menuKeyAction('ArrowUp', st(3, 0)), { type: 'move', index: 0 }, '先頭で止まる')

// checkout: ブランチがあるときだけ
assert.deepEqual(menuKeyAction('Enter', st(3, 1)), { type: 'checkout' }, 'Enter で checkout')
assert.equal(menuKeyAction('Enter', st(0, 0)), null, 'ブランチ 0 件で Enter は無効')

// 操作キー(大文字小文字両対応)
assert.deepEqual(menuKeyAction('p', st(1, 0)), { type: 'op', op: 'pull' }, 'p → pull')
assert.deepEqual(menuKeyAction('P', st(1, 0)), { type: 'op', op: 'pull' }, 'P → pull')
assert.deepEqual(menuKeyAction('u', st(1, 0)), { type: 'op', op: 'push' }, 'u → push')
assert.deepEqual(menuKeyAction('f', st(1, 0)), { type: 'op', op: 'fetch' }, 'f → fetch')

// 閉じる・対象外
assert.deepEqual(menuKeyAction('Escape', st(1, 0)), { type: 'close' }, 'Esc で閉じる')
assert.equal(menuKeyAction('x', st(1, 0)), null, '対象外キーは null')

// filterBranches: ブランチ名の部分一致(大文字小文字無視)
const bs = [
  { name: 'main', isCurrent: true, isRemote: false },
  { name: 'feature/login', isCurrent: false, isRemote: false },
  { name: 'feature/Logout', isCurrent: false, isRemote: true },
  { name: 'hotfix', isCurrent: false, isRemote: false },
]
assert.deepEqual(filterBranches(bs, ''), bs, '空クエリは全件')
assert.deepEqual(filterBranches(bs, '   '), bs, '空白のみは全件')
assert.deepEqual(
  filterBranches(bs, 'feature').map((b) => b.name),
  ['feature/login', 'feature/Logout'],
  '部分一致'
)
assert.deepEqual(
  filterBranches(bs, 'LOGOUT').map((b) => b.name),
  ['feature/Logout'],
  '大文字小文字を無視'
)
assert.deepEqual(filterBranches(bs, 'nomatch'), [], '一致なしは空配列')

console.log('gitMenu: OK')
