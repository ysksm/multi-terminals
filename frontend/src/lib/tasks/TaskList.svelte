<script>
  // タスク一覧(ホーム)。絞り込み・並び替え・ページングはフロントで行う(全件 1 リクエスト)。
  import {
    TASK_STATES,
    TASK_STATE_LABEL,
    countByState,
    countBySource,
    sourceOf,
    filterTasks,
    sortTasks,
    paginate,
    agentsForTask,
    relativeTime,
  } from '../tasks.js'
  import { STATE_GLYPH } from '../agentStatus.js'

  let {
    tasks = [],
    workspaces = [],
    agentPanes = {},
    busy = false,
    lastSync = '',
    localStatuses = [],
    onStatusChange = () => {},
    onOpen = () => {},
    onDetail = () => {},
    onImport = () => {},
    onSync = () => {},
    onSetup = () => {},
    onDeleteWorkDir = () => {},
    onResume = () => {},
  } = $props()

  let state = $state('all')
  let source = $state('all')
  let query = $state('')
  let sort = $state('state')
  let page = $state(1)
  let perPage = $state(20)
  let confirmingDelete = $state(null) // taskId

  const counts = $derived(countByState(tasks))
  const srcCounts = $derived(countBySource(tasks))
  const visible = $derived(sortTasks(filterTasks(tasks, { state, source, query }), sort))
  const pg = $derived(paginate(visible, page, perPage))

  // 絞り込み条件が変わったら 1 ページ目へ
  $effect(() => {
    state
    source
    query
    sort
    perPage
    page = 1
  })

  function repoGlyph(r) {
    return r.source?.kind === 'baseCopy' ? '⧉' : '⤓'
  }
</script>

<div class="toolbar">
  <strong>タスク</strong>
  <input class="search" placeholder="番号・概要・担当・ラベルで絞り込み" bind:value={query} />
  <span class="spacer"></span>
  {#if lastSync}<span class="muted">最終同期 {relativeTime(lastSync)}</span>{/if}
  <button onclick={onSync} disabled={busy} title="全タスクを Jira から再取得（JQL があれば自動追加）">⟳ Jira を同期</button>
  <button class="primary" onclick={onImport} disabled={busy}>＋ タスクを追加</button>
</div>

<div class="body">
  <div class="chips">
    <button class="chip" class:on={state === 'all'} onclick={() => (state = 'all')}>すべて <span class="n">{counts.all}</span></button>
    {#each TASK_STATES as s}
      <button class="chip" class:on={state === s} onclick={() => (state = s)}>
        <span class="dot {s}"></span>{TASK_STATE_LABEL[s]} <span class="n">{counts[s]}</span>
      </button>
    {/each}
    <span class="vsep"></span>
    <button class="chip" class:on={source === 'all'} onclick={() => (source = 'all')}>取込元: すべて</button>
    <button class="chip" class:on={source === 'jira'} onclick={() => (source = 'jira')}><span class="src jira">Jira</span> <span class="n">{srcCounts.jira}</span></button>
    <button class="chip" class:on={source === 'local'} onclick={() => (source = 'local')}><span class="src local">📝 ローカル</span> <span class="n">{srcCounts.local}</span></button>
    <span class="spacer"></span>
    <select bind:value={sort}>
      <option value="state">並び: ローカル状態 → 更新日時</option>
      <option value="updated">並び: 更新日時</option>
      <option value="priority">並び: 優先度</option>
      <option value="key">並び: Jira 番号</option>
    </select>
  </div>

  {#if tasks.length === 0}
    <div class="empty">
      <p>まだタスクがありません。</p>
      <p class="muted">「＋ タスクを追加」から手動で作成するか、右上の「⚙ 設定 → Jira 接続」を登録して Jira から取り込んでください。</p>
    </div>
  {:else if visible.length === 0}
    <div class="empty muted">条件に合うタスクはありません</div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>番号</th><th>概要</th><th>ステータス</th><th>ローカル</th><th>リポジトリ</th><th>ワークスペース</th><th>更新</th><th></th>
          </tr>
        </thead>
        <tbody>
          {#each pg.items as t (t.id)}
            {@const agents = agentsForTask(t, workspaces, agentPanes)}
            {@const local = sourceOf(t) === 'local'}
            <tr class="row" class:done={t.effectiveState === 'done'} class:local>
              <td class="key">
                <span class="src" class:jira={!local} class:local>{local ? '📝 ローカル' : 'Jira'}</span>
                <button class="link" class:local onclick={() => onDetail(t)}>{t.jiraKey}</button>
              </td>
              <td class="summary">
                <div class="sum">{t.jira?.summary || '（概要なし）'}</div>
                <div class="meta muted">
                  {#if local}
                    {#if t.jira?.priority}優先度: {t.jira.priority}{/if}
                    {#if t.local?.labels?.length}· ラベル: {t.local.labels.join(', ')}{/if}
                    {#if t.local?.links?.length}· 🔗 リンク {t.local.links.length}{/if}
                  {:else}
                    担当: {t.jira?.assignee || '未割当'}
                    {#if t.jira?.priority}· 優先度: {t.jira.priority}{/if}
                    {#if t.jira?.sprint}· {t.jira.sprint}{/if}
                  {/if}
                </div>
              </td>
              <td>
                {#if local}
                  <select class="status" value={t.local?.statusId || ''} onchange={(e) => onStatusChange(t, e.currentTarget.value)} disabled={busy}>
                    {#each localStatuses as st (st.id)}<option value={st.id}>{st.name}</option>{/each}
                  </select>
                {:else}
                  <span class="pill">{t.jira?.status || '—'}</span>
                {/if}
              </td>
              <td>
                <span class="state">
                  <span class="dot {t.effectiveState}"></span>{TASK_STATE_LABEL[t.effectiveState] || t.effectiveState}
                  {#if t.effectiveState === 'preparing'}<span class="spin">⟳</span>{/if}
                </span>
                {#each agents as a (a.tool)}
                  <span class="agent-badge" title="{a.tool}: 応答待ち {a.blocked} / 処理中 {a.working} / 完了 {a.done} / 入力待ち {a.idle}">
                    {a.tool}
                    {#if a.blocked}<span class="agent-st blocked">{STATE_GLYPH.blocked}{a.blocked}</span>{/if}
                    {#if a.working}<span class="agent-st working">{STATE_GLYPH.working}{a.working}</span>{/if}
                    {#if a.done}<span class="agent-st done">{STATE_GLYPH.done}{a.done}</span>{/if}
                  </span>
                {/each}
              </td>
              <td class="repos">
                {#if t.env?.repos?.length}
                  {#each t.env.repos as r (r.name)}
                    <span class="repo-tag" title={r.source?.kind === 'baseCopy' ? 'ベースクローンからコピー' : 'git clone'}>{repoGlyph(r)} {r.name}</span>
                  {/each}
                {:else if t.env?.workDir}
                  <span class="muted">作業フォルダのみ</span>
                {:else}
                  <span class="muted">未設定</span>
                {/if}
              </td>
              <td class="ws">
                {#if t.workspaceName}
                  <button class="link ws-link" onclick={() => onOpen(t)} title={t.workspaceName}>{t.workspaceName}</button>
                {:else if t.workspaceMissing}
                  <span class="muted">削除済み</span>
                {:else}
                  <span class="muted">—</span>
                {/if}
              </td>
              <td class="muted nowrap" title={t.updatedAt}>{relativeTime(t.updatedAt)}</td>
              <td class="actions">
                {#if t.effectiveState === 'working' || t.effectiveState === 'ready'}
                  {#if t.workspaceName}
                    <button class="primary sm" onclick={() => onOpen(t)}>開く</button>
                  {:else}
                    <button class="sm" onclick={() => onSetup(t)} disabled={busy}>▶ 作り直す</button>
                  {/if}
                {:else if t.effectiveState === 'none'}
                  <button class="primary sm" onclick={() => onSetup(t)} disabled={busy}>▶ 環境を作る</button>
                {:else if t.effectiveState === 'preparing'}
                  <button class="sm" onclick={() => onResume(t)}>進捗を見る</button>
                {:else if t.effectiveState === 'done'}
                  {#if confirmingDelete === t.id}
                    <button class="sm danger" onclick={() => { confirmingDelete = null; onDeleteWorkDir(t) }} disabled={busy}>削除する</button>
                    <button class="sm" onclick={() => (confirmingDelete = null)}>取消</button>
                  {:else if t.env?.workDir}
                    <button class="sm" onclick={() => (confirmingDelete = t.id)} title="作業フォルダと紐付くワークスペースを削除">🗑 作業フォルダを削除</button>
                  {/if}
                {/if}
                <button class="sm" onclick={() => onDetail(t)}>詳細</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="legend muted">
      <span><span class="src jira">Jira</span> = Jira から取り込み（読み取り専用）</span>
      <span><span class="src local">📝 ローカル</span> = 手動で作成（この画面で編集）</span>
      <span>⧉ ベースクローンからコピー</span>
      <span>⤓ git clone</span>
      <span><span class="dot working"></span>作業中 = ワークスペースが起動中</span>
      <span><span class="dot ready"></span>環境あり = フォルダとワークスペースが準備済み</span>
      <span class="spacer"></span>
      <span>{pg.from}–{pg.to} / {pg.total} 件</span>
      <button class="icon" disabled={pg.page <= 1} onclick={() => (page = pg.page - 1)}>‹</button>
      <button class="icon" disabled={pg.page >= pg.pages} onclick={() => (page = pg.page + 1)}>›</button>
      <select bind:value={perPage}>
        <option value={20}>20 件/頁</option>
        <option value={50}>50 件/頁</option>
        <option value={100}>100 件/頁</option>
      </select>
    </div>
  {/if}
</div>

<style>
  .toolbar .search { width: 260px; }
  .body { padding: 16px; overflow-y: auto; flex: 1; min-height: 0; }
  .chips { display: flex; gap: 6px; margin-bottom: 14px; flex-wrap: wrap; align-items: center; }
  .chip { display: inline-flex; align-items: center; gap: 6px; border-radius: 999px; padding: 3px 10px; font-size: 12px; background: var(--panel); }
  .chip.on { border-color: var(--accent); color: #fff; }
  .chip .n { font-size: 11px; color: var(--muted); }
  .chips select { width: auto; padding: 3px 8px; font-size: 12px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; flex-shrink: 0; background: var(--muted); }
  .dot.working { background: #22c55e; box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.18); }
  .dot.preparing { background: #f59e0b; }
  .dot.ready { background: var(--accent); }
  .dot.none { background: transparent; border: 1.5px solid var(--muted); }
  .dot.done { background: var(--muted); }
  .empty { padding: 40px 16px; text-align: center; }
  .empty p { margin: 4px 0; }
  .table-wrap { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th { text-align: left; font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); padding: 6px 10px; border-bottom: 1px solid var(--border); white-space: nowrap; }
  td { padding: 10px; border-bottom: 1px solid var(--border); vertical-align: top; }
  tr.done td { opacity: 0.7; }
  td.key { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 12px; white-space: nowrap; }
  td.key .src { margin-right: 4px; }
  tr.local td { background: rgba(167, 139, 250, 0.04); }
  .src { display: inline-flex; align-items: center; gap: 4px; font-size: 10px; border-radius: 8px; padding: 0 6px; border: 1px solid var(--border); white-space: nowrap; font-family: var(--sans); vertical-align: middle; }
  .src.jira { color: #4c9aff; border-color: rgba(76, 154, 255, 0.35); }
  .src.local { color: #c4b5fd; border-color: rgba(167, 139, 250, 0.4); background: rgba(167, 139, 250, 0.1); }
  .vsep { width: 1px; height: 18px; background: var(--border); margin: 0 4px; }
  select.status { width: auto; padding: 2px 6px; font-size: 12px; }
  .link { background: transparent; border-color: transparent; padding: 0; color: #4c9aff; font-family: inherit; }
  .link.local { color: #c4b5fd; }
  .link:hover:not(:disabled) { text-decoration: underline; border-color: transparent; }
  .ws-link { color: var(--text); font-size: 12px; max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: inline-block; text-align: left; }
  .summary .sum { color: #fff; }
  .summary .meta { font-size: 11px; margin-top: 2px; }
  .pill { font-size: 11px; border-radius: 4px; padding: 1px 6px; background: var(--panel-2); border: 1px solid var(--border); white-space: nowrap; }
  .state { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
  .spin { display: inline-block; animation: spin 1.2s linear infinite; color: #f59e0b; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .agent-badge { display: inline-flex; align-items: center; gap: 3px; font-size: 10px; color: var(--muted); border: 1px solid var(--border); border-radius: 8px; padding: 0 5px; margin-left: 6px; vertical-align: middle; }
  .agent-st.blocked { color: #ffb300; }
  .agent-st.working { color: #4caf50; }
  .agent-st.done { color: #7aa7ff; }
  .repo-tag { display: inline-block; font-family: ui-monospace, Menlo, monospace; font-size: 11px; border: 1px solid var(--border); border-radius: 4px; padding: 0 5px; margin: 0 4px 2px 0; white-space: nowrap; }
  .nowrap { white-space: nowrap; }
  td.actions { text-align: right; white-space: nowrap; }
  td.actions button { margin-left: 4px; }
  button.sm { padding: 3px 8px; font-size: 12px; }
  .legend { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; font-size: 11px; margin-top: 10px; }
  .legend .dot { width: 7px; height: 7px; box-shadow: none; margin-right: 4px; }
  .legend select { width: auto; padding: 1px 6px; font-size: 11px; }
</style>
