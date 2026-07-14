// エージェント(claude/codex)稼働状況の購読と、ワークスペース単位への集計。
// サーバは pane 単位で配信する(疎結合)ため、workspace への割り付けはここで行う。

// panes: { [paneId]: [{tool, state}] } / workspaces: [{id, panes: [{id}]}]
// 戻り値: Map(workspaceId → [{tool, active, wait}] ツール名昇順)。稼働ゼロの
// workspace はエントリを持たない。
export function aggregateByWorkspace(panes, workspaces) {
  const byWs = new Map()
  for (const ws of workspaces || []) {
    const counts = new Map()
    for (const pane of ws.panes || []) {
      for (const a of (panes || {})[pane.id] || []) {
        const c = counts.get(a.tool) || { tool: a.tool, active: 0, wait: 0 }
        if (a.state === 'wait') c.wait++
        else c.active++
        counts.set(a.tool, c)
      }
    }
    if (counts.size > 0) {
      byWs.set(ws.id, [...counts.values()].sort((x, y) => x.tool.localeCompare(y.tool)))
    }
  }
  return byWs
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
