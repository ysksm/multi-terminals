// 全ワークスペース横断で「アクティブなターミナル」を集める pure 関数群。
// サーバは live セッション(/api/sessions)もエージェント稼働状況(/api/agent-status)も
// pane 単位で配信する(疎結合)ため、ワークスペース情報との突き合わせはここで行う。

// 並び順の優先度。許可待ち(ユーザーの操作待ちで止まっている)を最上位に置く。
const STATUS_ORDER = { wait: 0, active: 1, idle: 2 }

// paneStatus は pane のエージェント一覧から表示上の状態を決める。
// wait が 1 つでもあれば wait、エージェントが居なければ idle(＝起動中なだけ)。
export function paneStatus(agents) {
  if (!agents || agents.length === 0) return 'idle'
  return agents.some((a) => a.state === 'wait') ? 'wait' : 'active'
}

/**
 * 起動中(ライブセッションを持つ)ペインを全ワークスペースから集めて 1 本のリストにする。
 *
 * @param {object} opts
 * @param {Array} opts.workspaces - /api/workspaces のレスポンス（pane 情報を含む）
 * @param {Set<string>|string[]} opts.livePaneIds - /api/sessions の paneIds
 * @param {Object} opts.agentPanes - paneId → [{tool, state}]（/api/agent-status）
 * @param {boolean} opts.agentOnly - true なら claude/codex 稼働中のペインだけに絞る
 * @returns {Array} 許可待ち → 実行中 → その他、同順位はワークスペース名・スロット順
 */
export function collectActiveTerminals({ workspaces, livePaneIds, agentPanes, agentOnly = false } = {}) {
  const live = livePaneIds instanceof Set ? livePaneIds : new Set(livePaneIds || [])
  const out = []
  for (const ws of workspaces || []) {
    for (const pane of ws.panes || []) {
      if (!live.has(pane.id)) continue
      const agents = [...((agentPanes || {})[pane.id] || [])].sort((a, b) => a.tool.localeCompare(b.tool))
      const status = paneStatus(agents)
      if (agentOnly && status === 'idle') continue
      out.push({
        paneId: pane.id,
        workspaceId: ws.id,
        workspaceName: ws.name || '',
        title: pane.title || '',
        directory: pane.directory || '',
        slot: pane.slot ?? 0,
        remoteHost: pane.remoteHost || '',
        agents,
        status,
      })
    }
  }
  out.sort(
    (a, b) =>
      STATUS_ORDER[a.status] - STATUS_ORDER[b.status] ||
      a.workspaceName.localeCompare(b.workspaceName) ||
      a.slot - b.slot ||
      a.paneId.localeCompare(b.paneId)
  )
  return out
}

/**
 * 件数に応じた可変グリッドの列数・行数。既存レイアウトプリセット(最大4ペイン)とは
 * 別物で、集約ビュー専用。列は正方形に近づけつつ maxCols で頭打ちにし、はみ出す分は
 * 行を増やす（表示側は縦スクロールさせる）。
 *
 * @param {number} count
 * @param {{maxCols?: number}} opts
 * @returns {{cols: number, rows: number}}
 */
export function gridDimensions(count, { maxCols = 4 } = {}) {
  const n = Math.max(0, Math.floor(count) || 0)
  if (n <= 1) return { cols: 1, rows: 1 }
  const cols = Math.min(maxCols, Math.ceil(Math.sqrt(n)))
  return { cols, rows: Math.ceil(n / cols) }
}
