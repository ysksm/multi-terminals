import { test } from 'node:test'
import assert from 'node:assert/strict'
import { aggregateByWorkspace } from './agentStatus.js'

test('pane 単位の状態を workspace 単位のツール別件数へ集計する', () => {
  const panes = {
    p1: [{ tool: 'claude', state: 'active' }],
    p2: [
      { tool: 'claude', state: 'wait' },
      { tool: 'codex', state: 'wait' },
    ],
    p9: [{ tool: 'claude', state: 'active' }], // どの workspace にも属さない
  }
  const workspaces = [
    { id: 'w1', panes: [{ id: 'p1' }, { id: 'p2' }] },
    { id: 'w2', panes: [{ id: 'p3' }] },
  ]
  const m = aggregateByWorkspace(panes, workspaces)
  assert.deepEqual(m.get('w1'), [
    { tool: 'claude', active: 1, wait: 1 },
    { tool: 'codex', active: 0, wait: 1 },
  ])
  assert.equal(m.has('w2'), false, '稼働ゼロの workspace はエントリなし')
})

test('空入力は空 Map', () => {
  assert.equal(aggregateByWorkspace({}, []).size, 0)
  assert.equal(aggregateByWorkspace(undefined, undefined).size, 0)
})
