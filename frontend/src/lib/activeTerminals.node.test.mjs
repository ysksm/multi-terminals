import assert from 'node:assert'
import { collectActiveTerminals, gridDimensions, paneStatus } from './activeTerminals.js'

// ---- paneStatus ----
assert.equal(paneStatus(undefined), 'idle', 'エージェント情報なし=idle')
assert.equal(paneStatus([]), 'idle', 'エージェント 0 件=idle')
assert.equal(paneStatus([{ tool: 'claude', state: 'active' }]), 'active', '実行中=active')
assert.equal(
  paneStatus([{ tool: 'claude', state: 'active' }, { tool: 'codex', state: 'wait' }]),
  'wait',
  'wait が 1 つでもあれば wait'
)

// ---- collectActiveTerminals ----
const workspaces = [
  {
    id: 'w2',
    name: 'beta',
    panes: [
      { id: 'p3', slot: 0, title: 'api', directory: '/b/api' },
      { id: 'p4', slot: 1, title: '', directory: '/b/web' },
    ],
  },
  {
    id: 'w1',
    name: 'alpha',
    panes: [
      { id: 'p1', slot: 0, title: 'shell', directory: '/a/one' },
      { id: 'p2', slot: 1, title: '', directory: '/a/two' },
    ],
  },
]

// 起動していない pane は出てこない
{
  const got = collectActiveTerminals({
    workspaces,
    livePaneIds: ['p1', 'p4'],
    agentPanes: {},
  })
  assert.deepEqual(got.map((t) => t.paneId), ['p1', 'p4'], 'live な pane だけを集める')
  assert.equal(got[0].workspaceName, 'alpha', 'ワークスペース名が付く')
  assert.equal(got[1].title, '', 'タイトル未設定は空文字')
  assert.equal(got[1].directory, '/b/web', 'ディレクトリが付く')
}

// live に居ないワークスペース(削除済み等)の pane は無視される
{
  const got = collectActiveTerminals({ workspaces, livePaneIds: ['nope'], agentPanes: {} })
  assert.deepEqual(got, [], '未知の paneId は無視')
}

// 並び順: 許可待ち → 実行中 → その他、同順位はワークスペース名 → スロット
{
  const got = collectActiveTerminals({
    workspaces,
    livePaneIds: new Set(['p1', 'p2', 'p3', 'p4']),
    agentPanes: {
      p1: [{ tool: 'claude', state: 'active' }],
      p3: [{ tool: 'codex', state: 'wait' }],
      p4: [{ tool: 'claude', state: 'active' }],
    },
  })
  assert.deepEqual(got.map((t) => t.paneId), ['p3', 'p1', 'p4', 'p2'], '許可待ち→実行中→idle の順')
  assert.deepEqual(got.map((t) => t.status), ['wait', 'active', 'active', 'idle'])
}

// agentOnly: エージェントが居ないペインを除外する
{
  const got = collectActiveTerminals({
    workspaces,
    livePaneIds: ['p1', 'p2', 'p3'],
    agentPanes: { p1: [{ tool: 'claude', state: 'active' }], p3: [{ tool: 'codex', state: 'wait' }] },
    agentOnly: true,
  })
  assert.deepEqual(got.map((t) => t.paneId), ['p3', 'p1'], 'エージェント稼働中のみに絞る')
}

// 複数ツールはツール名昇順に整列する
{
  const got = collectActiveTerminals({
    workspaces,
    livePaneIds: ['p1'],
    agentPanes: { p1: [{ tool: 'codex', state: 'active' }, { tool: 'claude', state: 'active' }] },
  })
  assert.deepEqual(got[0].agents.map((a) => a.tool), ['claude', 'codex'], 'ツール名昇順')
}

// 入力が空でも落ちない
assert.deepEqual(collectActiveTerminals(), [], '引数なしでも空配列')
assert.deepEqual(collectActiveTerminals({ workspaces: null, livePaneIds: null }), [], 'null 入力でも空配列')

// ---- gridDimensions ----
assert.deepEqual(gridDimensions(0), { cols: 1, rows: 1 }, '0 件でも 1x1')
assert.deepEqual(gridDimensions(1), { cols: 1, rows: 1 }, '1 件は 1x1')
assert.deepEqual(gridDimensions(2), { cols: 2, rows: 1 }, '2 件は左右')
assert.deepEqual(gridDimensions(3), { cols: 2, rows: 2 }, '3 件は 2 列 2 行')
assert.deepEqual(gridDimensions(4), { cols: 2, rows: 2 }, '4 件は 2x2')
assert.deepEqual(gridDimensions(6), { cols: 3, rows: 2 }, '6 件は 3 列 2 行')
assert.deepEqual(gridDimensions(9), { cols: 3, rows: 3 }, '9 件は 3x3')
assert.deepEqual(gridDimensions(20), { cols: 4, rows: 5 }, '列は maxCols で頭打ち')
assert.deepEqual(gridDimensions(20, { maxCols: 2 }), { cols: 2, rows: 10 }, 'maxCols を変更できる')

console.log('activeTerminals: all assertions passed')
