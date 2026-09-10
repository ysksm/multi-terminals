<script>
  // 環境セットアップの進捗。SSE(/api/setup-runs/{id}/stream)で更新を受け、
  // 失敗時は「このステップから再実行」「スキップして続行」「中止」を出す。
  import { onMount } from 'svelte'
  import { api } from '../api.js'
  import { formatDuration } from '../tasks.js'

  let {
    runId,
    task = null,
    onBack = () => {},
    onOpenWorkspace = () => {},
    onFinished = () => {},
    onError = () => {},
  } = $props()

  let run = $state(null)
  let busy = $state(false)
  let openLogs = $state(new Set())
  let es = null
  let pollTimer = null

  const progress = $derived(run ? run.steps.filter((s) => s.state === 'done' || s.state === 'skipped').length : 0)
  const total = $derived(run ? run.steps.length : 0)
  const terminal = $derived(run && ['done', 'failed', 'aborted'].includes(run.state))
  const failedIdx = $derived(run ? run.steps.findIndex((s) => s.state === 'failed') : -1)

  function apply(next) {
    const prev = run
    run = next
    // 失敗したステップのログは自動で開く
    for (const s of next.steps) if (s.state === 'failed') openLogs = new Set([...openLogs, s.index])
    if (['done', 'failed', 'aborted'].includes(next.state) && (!prev || !['done', 'failed', 'aborted'].includes(prev.state))) {
      onFinished(next)
    }
  }

  function connect() {
    disconnect()
    if (!runId) return
    try {
      es = new EventSource(`/api/setup-runs/${runId}/stream`)
      es.onmessage = (e) => {
        try {
          apply(JSON.parse(e.data))
        } catch {
          // 壊れたイベントは無視
        }
      }
      es.onerror = () => {
        // 終端で閉じられた場合も onerror が来る。終端でなければポーリングに切り替える。
        disconnect()
        if (!terminal) startPolling()
      }
    } catch {
      startPolling()
    }
  }

  function disconnect() {
    if (es) {
      es.close()
      es = null
    }
  }

  function startPolling() {
    clearInterval(pollTimer)
    pollTimer = setInterval(async () => {
      try {
        const r = await api.getSetupRun(runId)
        apply(r)
        if (['done', 'failed', 'aborted'].includes(r.state)) clearInterval(pollTimer)
      } catch {
        // 次回に任せる
      }
    }, 1000)
  }

  onMount(() => {
    api.getSetupRun(runId).then(apply).catch((e) => onError(e.message))
    connect()
    return () => {
      disconnect()
      clearInterval(pollTimer)
    }
  })

  // runId が変わったら購読し直す
  $effect(() => {
    runId
    run = null
    api.getSetupRun(runId).then(apply).catch((e) => onError(e.message))
    connect()
  })

  async function act(fn) {
    busy = true
    try {
      const r = await fn(runId)
      if (r && r.steps) apply(r)
      connect()
    } catch (e) {
      onError(e.message)
    } finally {
      busy = false
    }
  }

  function toggleLog(i) {
    const next = new Set(openLogs)
    if (next.has(i)) next.delete(i)
    else next.add(i)
    openLogs = next
  }

  const ICON = { pending: '○', running: '◐', done: '✓', failed: '✕', skipped: '↷' }
</script>

<div class="toolbar">
  <span class="crumb"><button class="link" onclick={onBack}>タスク</button> / {#if task}<button class="link" onclick={onBack}>{task.jiraKey}</button> /{/if}</span>
  <strong>環境セットアップ</strong>
  <span class="spacer"></span>
  {#if run && !terminal}
    <button onclick={onBack}>バックグラウンドで続行</button>
    <button class="danger" onclick={() => act(api.abortSetupRun)} disabled={busy}>中止</button>
  {:else}
    <button onclick={onBack}>戻る</button>
  {/if}
</div>

<div class="body">
  {#if !run}
    <p class="muted">読み込み中…</p>
  {:else}
    {#if run.state === 'done'}
      <div class="banner ok">
        <strong>セットアップ完了</strong>
        <span class="muted">{task ? `ワークスペース「${task.jiraKey} ${task.jira?.summary || ''}」を作成しました` : ''}</span>
        <span class="spacer"></span>
        {#if run.workspaceId}<button class="primary" onclick={() => onOpenWorkspace(run.workspaceId)}>▶ ワークスペースを開く</button>{/if}
      </div>
    {:else if run.state === 'failed'}
      <div class="banner bad">
        <strong>ステップ {failedIdx + 1} で失敗しました</strong>
        <span class="muted">{run.error}</span>
        <span class="spacer"></span>
        <button class="primary" onclick={() => act(api.retrySetupRun)} disabled={busy}>⟳ このステップから再実行</button>
        <button onclick={() => act(api.skipSetupStep)} disabled={busy}>↷ スキップして続行</button>
      </div>
    {:else if run.state === 'aborted'}
      <div class="banner warn">
        <strong>中止しました</strong>
        <span class="muted">作成済みのフォルダは残っています。再実行すると中断したステップから続けます。</span>
        <span class="spacer"></span>
        <button class="primary" onclick={() => act(api.retrySetupRun)} disabled={busy}>⟳ 再実行</button>
      </div>
    {/if}

    <div class="cols">
      <div class="card main">
        <header>
          <strong>{task ? `${task.jiraKey} ${task.jira?.summary || ''}` : run.taskId}</strong>
          <span class="spacer"></span>
          <span class="muted">{progress} / {total}</span>
        </header>
        <div class="bar"><div class="fill" class:bad={run.state === 'failed'} style="width: {total ? (progress / total) * 100 : 0}%"></div></div>
        <ol class="steps">
          {#each run.steps as s (s.index)}
            <li class="step {s.state}">
              <span class="ic" class:spin={s.state === 'running'}>{ICON[s.state] || '○'}</span>
              <span class="t">{s.label}</span>
              <span class="dur muted">{formatDuration(s.durationMs)}</span>
              <span class="sub"><code>{s.command}</code></span>
              {#if s.log}
                <button class="log-toggle" onclick={() => toggleLog(s.index)}>{openLogs.has(s.index) ? '▾ ログを隠す' : '▸ ログを表示'}</button>
                {#if openLogs.has(s.index)}
                  <pre class="log">{s.log}</pre>
                {/if}
              {/if}
            </li>
          {/each}
        </ol>
      </div>

      <div class="side">
        {#if task}
          <div class="card">
            <header><strong>この実行の内容</strong></header>
            <dl class="kv">
              <dt>作業フォルダ</dt><dd><code>{task.env?.workDir}</code></dd>
              <dt>ブランチ</dt><dd><code>{task.env?.branch || '（作成しない）'}</code></dd>
              <dt>リポジトリ</dt><dd>{#if task.env?.repos?.length}{#each task.env.repos as r (r.name)}<div>{r.name}（{r.source.kind === 'baseCopy' ? '⧉ ベースからコピー' : '⤓ git clone'}）</div>{/each}{:else}<span class="muted">なし</span>{/if}</dd>
              <dt>レイアウト</dt><dd>{task.env?.layout}</dd>
            </dl>
          </div>
        {/if}
        <div class="card">
          <header><strong>失敗時の挙動</strong></header>
          <p class="muted note">失敗したステップで停止し、出力をそのまま表示します。「このステップから再実行」「スキップして続行」を選べます。作成済みのフォルダは残します（消したい場合はタスクの「作業フォルダを削除」）。</p>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .crumb { color: var(--muted); }
  .link { background: transparent; border-color: transparent; padding: 0; color: var(--text); }
  .link:hover:not(:disabled) { color: var(--accent); border-color: transparent; }
  .body { padding: 16px; overflow-y: auto; flex: 1; min-height: 0; }
  .banner { display: flex; align-items: center; gap: 12px; border-radius: 8px; padding: 12px 14px; margin-bottom: 12px; border: 1px solid var(--border); flex-wrap: wrap; }
  .banner.ok { border-color: rgba(34, 197, 94, 0.4); background: rgba(34, 197, 94, 0.12); }
  .banner.ok strong { color: #6ee7a0; }
  .banner.bad { border-color: rgba(239, 68, 68, 0.4); background: rgba(239, 68, 68, 0.12); }
  .banner.bad strong { color: #f87171; }
  .banner.warn { border-color: rgba(245, 158, 11, 0.4); background: rgba(245, 158, 11, 0.12); }
  .banner.warn strong { color: #fbbf24; }
  .cols { display: grid; grid-template-columns: minmax(360px, 2fr) minmax(240px, 1fr); gap: 16px; align-items: start; }
  .side { display: flex; flex-direction: column; gap: 16px; }
  .card { border: 1px solid var(--border); border-radius: 8px; background: var(--panel); }
  .card > header { display: flex; align-items: center; gap: 8px; padding: 10px 14px; border-bottom: 1px solid var(--border); }
  .card > header strong { color: #fff; font-size: 13px; }
  .bar { height: 4px; background: var(--panel-2); }
  .bar .fill { height: 100%; background: var(--accent); transition: width 0.3s; }
  .bar .fill.bad { background: var(--danger); }
  .steps { list-style: none; margin: 0; padding: 8px 0; }
  .step { display: grid; grid-template-columns: 24px 1fr auto; column-gap: 8px; padding: 8px 14px; border-bottom: 1px solid var(--border); align-items: center; }
  .step:last-child { border-bottom: 0; }
  .step .ic { text-align: center; color: var(--muted); }
  .step.done .ic { color: #22c55e; }
  .step.failed .ic { color: var(--danger); }
  .step.running .ic { color: var(--accent); }
  .step.skipped .ic { color: #f59e0b; }
  .step.pending .t { color: var(--muted); }
  .step .t { color: #fff; }
  .step .dur { font-size: 11px; font-variant-numeric: tabular-nums; }
  .step .sub { grid-column: 2 / -1; font-size: 11px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .step code { font-family: ui-monospace, Menlo, monospace; }
  .log-toggle { grid-column: 2 / -1; justify-self: start; background: transparent; border-color: transparent; font-size: 11px; color: var(--muted); padding: 2px 0; margin-top: 2px; }
  .log-toggle:hover:not(:disabled) { color: var(--text); border-color: transparent; }
  .log { grid-column: 2 / -1; margin: 4px 0 0; background: #0b0d10; border: 1px solid var(--border); border-radius: 6px; padding: 8px 10px; font-family: ui-monospace, Menlo, monospace; font-size: 11.5px; line-height: 1.5; white-space: pre-wrap; word-break: break-all; max-height: 260px; overflow: auto; color: #b7bec8; }
  .spin { display: inline-block; animation: spin 1.2s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .kv { display: grid; grid-template-columns: 96px 1fr; gap: 6px 10px; margin: 0; padding: 12px 14px; font-size: 12px; }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; min-width: 0; word-break: break-all; }
  .note { margin: 0; padding: 12px 14px; font-size: 12px; line-height: 1.5; }
</style>
