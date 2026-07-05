import assert from 'node:assert'
import { cycleWorkspaceId, workspaceIdAt } from './workspaceNav.js'

const ws = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]

// cycleWorkspaceId: 前後移動（巡回）
assert.equal(cycleWorkspaceId(ws, 'a', 1), 'b', 'next: a -> b')
assert.equal(cycleWorkspaceId(ws, 'b', 1), 'c', 'next: b -> c')
assert.equal(cycleWorkspaceId(ws, 'c', 1), 'a', 'next wraps: c -> a')
assert.equal(cycleWorkspaceId(ws, 'b', -1), 'a', 'prev: b -> a')
assert.equal(cycleWorkspaceId(ws, 'a', -1), 'c', 'prev wraps: a -> c')

// cycleWorkspaceId: 未選択・不明 id は先頭へ
assert.equal(cycleWorkspaceId(ws, null, 1), 'a', 'no current: first')
assert.equal(cycleWorkspaceId(ws, 'zzz', -1), 'a', 'unknown current: first')

// cycleWorkspaceId: 要素 1 つは自分自身
assert.equal(cycleWorkspaceId([{ id: 'x' }], 'x', 1), 'x', 'single: next=self')
assert.equal(cycleWorkspaceId([{ id: 'x' }], 'x', -1), 'x', 'single: prev=self')

// cycleWorkspaceId: 空リスト
assert.equal(cycleWorkspaceId([], 'a', 1), null, 'empty: null')
assert.equal(cycleWorkspaceId(null, 'a', 1), null, 'nullish list: null')

// workspaceIdAt: Cmd+1〜9 の 0-based ジャンプ
assert.equal(workspaceIdAt(ws, 0), 'a', 'at 0 = a')
assert.equal(workspaceIdAt(ws, 2), 'c', 'at 2 = c')
assert.equal(workspaceIdAt(ws, 3), null, 'out of range: null')
assert.equal(workspaceIdAt(ws, -1), null, 'negative: null')
assert.equal(workspaceIdAt([], 0), null, 'empty: null')
assert.equal(workspaceIdAt(null, 0), null, 'nullish list: null')

console.log('workspaceNav: OK')
