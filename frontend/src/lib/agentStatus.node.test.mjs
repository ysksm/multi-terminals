import { test } from 'node:test'
import assert from 'node:assert/strict'
import { aggregateByWorkspace, markSeen, nextDoneSet, withDone } from './agentStatus.js'

test('pane 単位の状態を workspace 単位のツール別件数へ集計する', () => {
  const panes = {
    p1: [{ tool: 'claude', state: 'working' }],
    p2: [
      { tool: 'claude', state: 'blocked' },
      { tool: 'codex', state: 'idle' },
    ],
    p9: [{ tool: 'claude', state: 'working' }], // どの workspace にも属さない
  }
  const workspaces = [
    { id: 'w1', panes: [{ id: 'p1' }, { id: 'p2' }] },
    { id: 'w2', panes: [{ id: 'p3' }] },
  ]
  const m = aggregateByWorkspace(panes, workspaces)
  assert.deepEqual(m.get('w1'), [
    { tool: 'claude', blocked: 1, working: 1, done: 0, idle: 0, unknown: 0 },
    { tool: 'codex', blocked: 0, working: 0, done: 0, idle: 1, unknown: 0 },
  ])
  assert.equal(m.has('w2'), false, '稼働ゼロの workspace はエントリなし')
})

test('未知の状態は unknown に数える', () => {
  const m = aggregateByWorkspace({ p1: [{ tool: 'claude', state: 'weird' }] }, [{ id: 'w', panes: [{ id: 'p1' }] }])
  assert.equal(m.get('w')[0].unknown, 1)
})

test('空入力は空 Map', () => {
  assert.equal(aggregateByWorkspace({}, []).size, 0)
  assert.equal(aggregateByWorkspace(undefined, undefined).size, 0)
})

test('nextDoneSet: 見ていないペインが working → idle になったら done', () => {
  const prev = { p1: [{ tool: 'claude', state: 'working' }], p2: [{ tool: 'codex', state: 'working' }] }
  const next = { p1: [{ tool: 'claude', state: 'idle' }], p2: [{ tool: 'codex', state: 'idle' }] }
  const done = nextDoneSet(new Set(), prev, next, 'p2')
  assert.deepEqual([...done], ['p1'], 'アクティブな p2 は done にしない')
})

test('nextDoneSet: 再び動き出した/消えた/見た ペインは done から外れる', () => {
  const done = new Set(['a', 'b', 'c'])
  const next = { a: [{ tool: 'claude', state: 'working' }], b: [{ tool: 'claude', state: 'idle' }] }
  const got = nextDoneSet(done, {}, next, 'b')
  assert.deepEqual([...got], [], 'a は working、b はアクティブ、c は消えた')
  const kept = nextDoneSet(new Set(['b']), {}, next, null)
  assert.deepEqual([...kept], ['b'], 'idle のまま見ていなければ維持')
})

test('nextDoneSet: blocked → idle も done 扱い、idle → idle は done にしない', () => {
  assert.deepEqual([...nextDoneSet(new Set(), { p: [{ tool: 'c', state: 'blocked' }] }, { p: [{ tool: 'c', state: 'idle' }] }, null)], ['p'])
  assert.deepEqual([...nextDoneSet(new Set(), { p: [{ tool: 'c', state: 'idle' }] }, { p: [{ tool: 'c', state: 'idle' }] }, null)], [])
})

test('markSeen / withDone', () => {
  const done = new Set(['p1'])
  assert.equal(markSeen(done, 'p2'), done, '変化なしなら同じ Set')
  assert.deepEqual([...markSeen(done, 'p1')], [])
  const panes = { p1: [{ tool: 'claude', state: 'idle' }], p2: [{ tool: 'codex', state: 'idle' }] }
  const got = withDone(panes, done)
  assert.equal(got.p1[0].state, 'done')
  assert.equal(got.p2[0].state, 'idle')
  assert.equal(withDone(panes, new Set()), panes, 'done が空なら同じオブジェクト')
})
