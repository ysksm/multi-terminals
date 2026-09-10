import assert from 'node:assert'
import {
  parseJiraKeys,
  countByState,
  countBySource,
  sourceOf,
  filterTasks,
  sortTasks,
  compareKeys,
  paginate,
  sidebarTasks,
  agentsForTask,
  relativeTime,
  formatDuration,
  formatBytes,
  parseHash,
  buildHash,
} from './tasks.js'

// parseJiraKeys
assert.deepEqual(parseJiraKeys('PROJ-1301, proj-1302 PROJ-1301'), ['PROJ-1301', 'PROJ-1302'], 'カンマ区切り・小文字・重複')
assert.deepEqual(parseJiraKeys('https://acme.atlassian.net/browse/PROJ-9?x=1'), ['PROJ-9'], 'URL から抽出')
assert.deepEqual(parseJiraKeys(''), [], '空')
assert.deepEqual(parseJiraKeys('1234-PROJ'), [], '数字始まりは番号でない')

const t = (key, state, extra = {}) => ({
  jiraKey: key,
  effectiveState: state,
  updatedAt: '2026-09-10T00:00:00Z',
  jira: { summary: `${key} の概要`, priority: 'Medium' },
  env: { branch: '' },
  ...extra,
})
const tasks = [
  t('PROJ-2', 'none', { updatedAt: '2026-09-09T00:00:00Z' }),
  t('PROJ-10', 'working'),
  t('PROJ-3', 'done'),
  t('PROJ-4', 'ready', { jira: { summary: 'ログ JSON 化', priority: 'High', assignee: '笠松' } }),
  t('PROJ-5', 'preparing'),
  t('PROJ-6', 'ready', { updatedAt: '2026-09-11T00:00:00Z' }),
]

// countByState
const c = countByState(tasks)
assert.equal(c.all, 6)
assert.equal(c.ready, 2)
assert.equal(c.working, 1)

// filterTasks
assert.equal(filterTasks(tasks, { state: 'ready' }).length, 2)
assert.deepEqual(
  filterTasks(tasks, { query: 'json' }).map((x) => x.jiraKey),
  ['PROJ-4'],
  '概要の部分一致(大小無視)'
)
assert.deepEqual(filterTasks(tasks, { query: '笠松' }).map((x) => x.jiraKey), ['PROJ-4'], '担当で検索')

// source
const mixed = [...tasks, t('T-001', 'none', { source: 'local', local: { labels: ['docs', 'dx'], links: [], statusId: 'todo' } })]
assert.equal(sourceOf(mixed[0]), 'jira', '空の source は jira')
assert.equal(sourceOf(mixed[mixed.length - 1]), 'local')
assert.deepEqual(countBySource(mixed), { all: 7, jira: 6, local: 1 })
assert.deepEqual(filterTasks(mixed, { source: 'local' }).map((x) => x.jiraKey), ['T-001'], '取込元で絞り込み')
assert.deepEqual(filterTasks(mixed, { query: 'docs' }).map((x) => x.jiraKey), ['T-001'], 'ラベルで検索')

// sortTasks
assert.deepEqual(
  sortTasks(tasks).map((x) => x.jiraKey),
  ['PROJ-10', 'PROJ-5', 'PROJ-6', 'PROJ-4', 'PROJ-2', 'PROJ-3'],
  '状態順 → 更新日時降順 → 番号'
)
assert.deepEqual(sortTasks(tasks, 'key').map((x) => x.jiraKey), ['PROJ-2', 'PROJ-3', 'PROJ-4', 'PROJ-5', 'PROJ-6', 'PROJ-10'], '番号は数値順')
assert.equal(sortTasks(tasks, 'priority')[0].jiraKey, 'PROJ-4', '優先度 High が先頭')
assert.equal(sortTasks(tasks, 'updated')[0].jiraKey, 'PROJ-6', '更新日時降順')
assert.equal(tasks[0].jiraKey, 'PROJ-2', '入力を変更しない')

// compareKeys
assert.ok(compareKeys('PROJ-9', 'PROJ-10') < 0)
assert.ok(compareKeys('ABC-1', 'PROJ-1') < 0)

// paginate
const pg = paginate(tasks, 2, 4)
assert.deepEqual([pg.page, pg.pages, pg.total, pg.from, pg.to, pg.items.length], [2, 2, 6, 5, 6, 2])
assert.equal(paginate(tasks, 99, 4).page, 2, '範囲外は最終ページ')
assert.deepEqual([paginate([], 1, 20).from, paginate([], 1, 20).to], [0, 0])

// sidebarTasks
assert.deepEqual(sidebarTasks(tasks, 2).map((x) => x.jiraKey), ['PROJ-10', 'PROJ-5'], '進行中だけ上限まで')
assert.ok(!sidebarTasks(tasks).some((x) => x.effectiveState === 'done'), '完了は出さない')

// agentsForTask
const ws = [{ id: 'w1', panes: [{ id: 'p1' }, { id: 'p2' }] }]
const agents = { p1: [{ tool: 'claude', state: 'working' }], p2: [{ tool: 'claude', state: 'blocked' }, { tool: 'codex', state: 'idle' }] }
assert.deepEqual(agentsForTask({ workspaceId: 'w1' }, ws, agents), [
  { tool: 'claude', blocked: 1, working: 1, done: 0, idle: 0 },
  { tool: 'codex', blocked: 0, working: 0, done: 0, idle: 1 },
])
assert.deepEqual(agentsForTask({ workspaceId: '' }, ws, agents), [])

// relativeTime
const now = Date.parse('2026-09-10T12:00:00Z')
assert.equal(relativeTime('2026-09-10T11:57:00Z', now), '3分前')
assert.equal(relativeTime('2026-09-09T12:00:00Z', now), '昨日')
assert.equal(relativeTime('2026-09-02T12:00:00Z', now), '8日前')
assert.equal(relativeTime('bad', now), '')

// formatDuration / formatBytes
assert.equal(formatDuration(4200), '4.2s')
assert.equal(formatDuration(72000), '1m 12s')
assert.equal(formatBytes(412 * 1024 * 1024), '412 MB')
assert.equal(formatBytes(0), '—')

// parseHash / buildHash
assert.deepEqual(parseHash(''), { screen: 'workspace' })
assert.deepEqual(parseHash('#tasks'), { screen: 'tasks' })
assert.deepEqual(parseHash('#tasks/proj-1'), { screen: 'task', key: 'PROJ-1' })
assert.deepEqual(parseHash('#tasks/PROJ-1/setup'), { screen: 'setup', key: 'PROJ-1' })
assert.deepEqual(parseHash('#settings'), { screen: 'settings', panel: 'templates' })
assert.deepEqual(parseHash('#settings/jira'), { screen: 'settings', panel: 'jira' })
for (const h of ['#ws', '#tasks', '#tasks/PROJ-1', '#tasks/PROJ-1/setup', '#settings/base']) {
  assert.equal(buildHash(parseHash(h)), h, `往復 ${h}`)
}

console.log('tasks.js: all assertions passed')
