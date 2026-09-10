// タスク管理画面の純関数群(表示用の整形・絞り込み・並び替え・ページング)。
// サーバの TaskDTO(/api/tasks)を入力にし、DOM や fetch に依存しない。

// 表示状態の一覧。effectiveState(サーバが導出)と同じ語彙。
export const TASK_STATES = ['working', 'preparing', 'ready', 'none', 'done']

export const TASK_STATE_LABEL = {
  working: '作業中',
  preparing: '準備中',
  ready: '環境あり',
  none: '未準備',
  done: '完了',
}

// サイドバーに出す状態(進行中)と上限件数
export const SIDEBAR_STATES = ['working', 'preparing', 'ready']
export const SIDEBAR_MAX = 5

const KEY_RE = /[A-Za-z][A-Za-z0-9_]*-\d+/g

/**
 * 自由入力(カンマ区切り・URL 貼り付け)から Jira 番号を抽出し、大文字化して重複を除く。
 * サーバ側 taskmgmt.ParseJiraKeys と同じ規則。
 */
export function parseJiraKeys(text) {
  const seen = new Set()
  const out = []
  for (const m of String(text || '').match(KEY_RE) || []) {
    const k = m.toUpperCase()
    if (!/^[A-Z][A-Z0-9_]*-\d+$/.test(k) || seen.has(k)) continue
    seen.add(k)
    out.push(k)
  }
  return out
}

/** 状態ごとの件数(チップ表示用)。all も含む。 */
export function countByState(tasks) {
  const c = { all: 0 }
  for (const s of TASK_STATES) c[s] = 0
  for (const t of tasks || []) {
    c.all++
    const s = t.effectiveState in c ? t.effectiveState : 'none'
    c[s]++
  }
  return c
}

/** 取込元。空(旧データ)は jira。 */
export function sourceOf(t) {
  return t?.source === 'local' ? 'local' : 'jira'
}

/** 取込元ごとの件数。 */
export function countBySource(tasks) {
  const c = { all: 0, jira: 0, local: 0 }
  for (const t of tasks || []) {
    c.all++
    c[sourceOf(t)]++
  }
  return c
}

/**
 * 絞り込み。state は 'all' か TASK_STATES の 1 つ、source は 'all' | 'jira' | 'local'。
 * query は番号・概要・担当・ブランチ・ラベルの部分一致。
 */
export function filterTasks(tasks, { state = 'all', source = 'all', query = '' } = {}) {
  const q = String(query || '').trim().toLowerCase()
  return (tasks || []).filter((t) => {
    if (state !== 'all' && t.effectiveState !== state) return false
    if (source !== 'all' && sourceOf(t) !== source) return false
    if (!q) return true
    const hay = [t.jiraKey, t.jira?.summary, t.jira?.assignee, t.env?.branch, t.jira?.status, ...(t.local?.labels || [])]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
}

const STATE_RANK = Object.fromEntries(TASK_STATES.map((s, i) => [s, i]))

const PRIORITY_RANK = { highest: 0, high: 1, medium: 2, low: 3, lowest: 4 }

/**
 * 並び替え。sort は 'state'(既定: 状態 → 更新日時降順) / 'updated' / 'priority' / 'key'。
 * 入力配列は変更しない。
 */
export function sortTasks(tasks, sort = 'state') {
  const arr = [...(tasks || [])]
  const byUpdated = (a, b) => String(b.updatedAt || '').localeCompare(String(a.updatedAt || ''))
  const byKey = (a, b) => compareKeys(a.jiraKey, b.jiraKey)
  switch (sort) {
    case 'updated':
      arr.sort((a, b) => byUpdated(a, b) || byKey(a, b))
      break
    case 'priority':
      arr.sort((a, b) => {
        const pa = PRIORITY_RANK[String(a.jira?.priority || '').toLowerCase()] ?? 9
        const pb = PRIORITY_RANK[String(b.jira?.priority || '').toLowerCase()] ?? 9
        return pa - pb || byUpdated(a, b) || byKey(a, b)
      })
      break
    case 'key':
      arr.sort(byKey)
      break
    default:
      arr.sort(
        (a, b) =>
          (STATE_RANK[a.effectiveState] ?? 9) - (STATE_RANK[b.effectiveState] ?? 9) ||
          byUpdated(a, b) ||
          byKey(a, b)
      )
  }
  return arr
}

/** "PROJ-12" と "PROJ-9" を数値で比べる。プロジェクトが違えば文字列比較。 */
export function compareKeys(a, b) {
  const [pa, na] = splitKey(a)
  const [pb, nb] = splitKey(b)
  if (pa !== pb) return pa.localeCompare(pb)
  return na - nb
}

function splitKey(k) {
  const i = String(k || '').lastIndexOf('-')
  if (i < 0) return [String(k || ''), 0]
  return [k.slice(0, i), Number(k.slice(i + 1)) || 0]
}

/**
 * ページング。page は 1 始まり。範囲外は最終ページに丸める。
 * @returns {{items: any[], page: number, pages: number, total: number, from: number, to: number}}
 */
export function paginate(items, page = 1, perPage = 20) {
  const total = (items || []).length
  const per = Math.max(1, perPage | 0)
  const pages = Math.max(1, Math.ceil(total / per))
  const p = Math.min(Math.max(1, page | 0), pages)
  const from = total === 0 ? 0 : (p - 1) * per + 1
  const to = Math.min(total, p * per)
  return { items: (items || []).slice((p - 1) * per, p * per), page: p, pages, total, from, to }
}

/**
 * サイドバー用: 進行中(作業中 → 準備中 → 環境あり)を上限件数まで。
 */
export function sidebarTasks(tasks, max = SIDEBAR_MAX) {
  return sortTasks(
    (tasks || []).filter((t) => SIDEBAR_STATES.includes(t.effectiveState)),
    'state'
  ).slice(0, max)
}

/**
 * タスクに紐付くワークスペースの pane で稼働中のエージェントをツール別に集計する。
 * agentPanes は /api/agent-status の paneId → [{tool, state}]。
 * @returns {Array<{tool: string, blocked: number, working: number, done: number, idle: number}>}
 */
export function agentsForTask(task, workspaces, agentPanes) {
  if (!task?.workspaceId) return []
  const ws = (workspaces || []).find((w) => w.id === task.workspaceId)
  if (!ws) return []
  const by = new Map()
  for (const p of ws.panes || []) {
    for (const a of (agentPanes || {})[p.id] || []) {
      const cur = by.get(a.tool) || { tool: a.tool, blocked: 0, working: 0, done: 0, idle: 0 }
      if (a.state in cur) cur[a.state]++
      by.set(a.tool, cur)
    }
  }
  return [...by.values()].sort((a, b) => a.tool.localeCompare(b.tool))
}

/** ISO 日時を「3分前」「昨日」のような相対表記にする。無効なら ''。 */
export function relativeTime(iso, now = Date.now()) {
  const t = Date.parse(iso)
  if (!iso || Number.isNaN(t)) return ''
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 60) return 'たった今'
  const m = Math.round(s / 60)
  if (m < 60) return `${m}分前`
  const h = Math.round(m / 60)
  if (h < 24) return `${h}時間前`
  const d = Math.round(h / 24)
  if (d === 1) return '昨日'
  if (d < 30) return `${d}日前`
  const mo = Math.round(d / 30)
  if (mo < 12) return `${mo}ヶ月前`
  return `${Math.round(mo / 12)}年前`
}

/** ミリ秒を "4.2s" / "1m 12s" 表記に。 */
export function formatDuration(ms) {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${(ms / 1000).toFixed(1)}s`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${Math.round(s - m * 60)}s`
}

/** バイト数を "412 MB" 表記に。 */
export function formatBytes(n) {
  if (!n || n <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
}

/**
 * URL ハッシュを画面へ解決する。
 *   ''/'#ws'            → {screen:'workspace'}
 *   '#tasks'            → {screen:'tasks'}
 *   '#tasks/PROJ-1'     → {screen:'task', key:'PROJ-1'}
 *   '#tasks/PROJ-1/setup' → {screen:'setup', key:'PROJ-1'}
 *   '#settings/jira'    → {screen:'settings', panel:'jira'}
 */
export function parseHash(hash) {
  const parts = String(hash || '')
    .replace(/^#/, '')
    .split('/')
    .filter(Boolean)
  switch (parts[0]) {
    case 'tasks':
      if (parts[1] && parts[2] === 'setup') return { screen: 'setup', key: parts[1].toUpperCase() }
      if (parts[1]) return { screen: 'task', key: parts[1].toUpperCase() }
      return { screen: 'tasks' }
    case 'settings':
      return { screen: 'settings', panel: parts[1] || 'templates' }
    default:
      return { screen: 'workspace' }
  }
}

/** parseHash の逆。 */
export function buildHash(route) {
  switch (route?.screen) {
    case 'tasks':
      return '#tasks'
    case 'task':
      return `#tasks/${route.key}`
    case 'setup':
      return `#tasks/${route.key}/setup`
    case 'settings':
      return `#settings/${route.panel || 'templates'}`
    default:
      return '#ws'
  }
}
