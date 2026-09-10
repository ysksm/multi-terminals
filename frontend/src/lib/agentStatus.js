// エージェント(claude/codex)稼働状況の購読と、ワークスペース単位への集計。
// サーバは pane 単位で配信する(疎結合)ため、workspace への割り付けはここで行う。
//
// サーバの状態は herdr と同じ 4 値: working(処理中) / blocked(許可・質問待ち) /
// idle(入力待ち) / unknown(判定不能)。これに加えてフロント側で、見ていない
// ペインが working → idle に変わったときに done(完了・未確認)を付ける。

// 表示上の状態一覧(並び順・ラベル・記号)。blocked は人の応答が必要なので最上位。
export const AGENT_STATES = ['blocked', 'working', 'done', 'idle', 'unknown']
export const STATE_LABEL = {
  blocked: '応答待ち',
  working: '処理中',
  done: '完了(未確認)',
  idle: '入力待ち',
  unknown: '不明',
}
export const STATE_GLYPH = { blocked: '⏸', working: '●', done: '✓', idle: '○', unknown: '?' }

// panes: { [paneId]: [{tool, state}] } / workspaces: [{id, panes: [{id}]}]
// 戻り値: Map(workspaceId → [{tool, blocked, working, done, idle, unknown}] ツール名昇順)。
// 稼働ゼロの workspace はエントリを持たない。
export function aggregateByWorkspace(panes, workspaces) {
  const byWs = new Map()
  for (const ws of workspaces || []) {
    const counts = new Map()
    for (const pane of ws.panes || []) {
      for (const a of (panes || {})[pane.id] || []) {
        const c = counts.get(a.tool) || { tool: a.tool, blocked: 0, working: 0, done: 0, idle: 0, unknown: 0 }
        c[AGENT_STATES.includes(a.state) ? a.state : 'unknown']++
        counts.set(a.tool, c)
      }
    }
    if (counts.size > 0) {
      byWs.set(ws.id, [...counts.values()].sort((x, y) => x.tool.localeCompare(y.tool)))
    }
  }
  return byWs
}

const busy = (agents) => (agents || []).some((a) => a.state === 'working' || a.state === 'blocked')
const settled = (agents) => (agents || []).length > 0 && (agents || []).every((a) => a.state === 'idle')

/**
 * done(完了・未確認)ペインの集合を更新する pure 関数。
 * 前回 working/blocked だったペインが idle に落ち着き、かつそのペインを
 * 見ていない(activePaneId でない)なら done に入れる。見た(activePaneId になった)
 * か、再び動き出した/消えたら外す。
 *
 * @param {Set<string>} done - 前回の集合(変更しない)
 * @param {Object} prevPanes - 前回の paneId → agents
 * @param {Object} nextPanes - 今回の paneId → agents
 * @param {string|null} activePaneId
 * @returns {Set<string>}
 */
export function nextDoneSet(done, prevPanes, nextPanes, activePaneId) {
  const out = new Set()
  for (const paneId of done || []) {
    if (paneId !== activePaneId && settled((nextPanes || {})[paneId])) out.add(paneId)
  }
  for (const [paneId, agents] of Object.entries(nextPanes || {})) {
    if (paneId === activePaneId) continue
    if (busy((prevPanes || {})[paneId]) && settled(agents)) out.add(paneId)
  }
  return out
}

/** activePaneId のペインを done から外した新しい集合を返す(変化が無ければ同じ Set)。 */
export function markSeen(done, activePaneId) {
  if (!done || !done.has(activePaneId)) return done
  const out = new Set(done)
  out.delete(activePaneId)
  return out
}

/** done に入っているペインの idle を 'done' に置き換えた panes を返す。 */
export function withDone(panes, done) {
  if (!done || done.size === 0) return panes || {}
  const out = {}
  for (const [paneId, agents] of Object.entries(panes || {})) {
    out[paneId] = done.has(paneId)
      ? agents.map((a) => (a.state === 'idle' ? { ...a, state: 'done' } : a))
      : agents
  }
  return out
}

// SSE(/api/agent-status/stream)で購読し、使えない環境(SSE 非対応・接続断)では
// /api/agent-status の定期ポーリングにフォールバックする。
// onUpdate(panes) を受信のたびに呼ぶ。戻り値は購読停止関数。
export function connectAgentStatus(onUpdate, { pollMs = 3000 } = {}) {
  let stopped = false
  let es = null
  let timer = null

  const poll = async () => {
    if (stopped) return
    try {
      const res = await fetch('/api/agent-status')
      if (res.ok) onUpdate((await res.json()).panes || {})
    } catch {
      // サーバ再起動中など。次回のポーリングに任せる。
    }
    timer = setTimeout(poll, pollMs)
  }

  try {
    es = new EventSource('/api/agent-status/stream')
    es.onmessage = (ev) => {
      try {
        onUpdate(JSON.parse(ev.data).panes || {})
      } catch {
        // 壊れたイベントは読み飛ばす
      }
    }
    es.onerror = () => {
      // SSE が張れない環境(Wails の AssetServer 等)はポーリングへ切替。
      es?.close()
      es = null
      if (!timer) poll()
    }
  } catch {
    poll()
  }

  return () => {
    stopped = true
    es?.close()
    if (timer) clearTimeout(timer)
  }
}
