<script>
  // タスク詳細: 左に Jira 情報(読み取り専用)+ ローカルメモ、右に作業環境の編集。
  import { api, layoutOf } from '../api.js'
  import { TASK_STATE_LABEL, relativeTime, sourceOf } from '../tasks.js'
  import EnvEditor from './EnvEditor.svelte'

  let {
    task,
    templates = [],
    baseClones = [],
    localStatuses = [],
    busy = false,
    onBack = () => {},
    onOpenWorkspace = () => {},
    onSetup = () => {},
    onResume = () => {},
    onChanged = () => {},
    onDeleted = () => {},
    onDeleteWorkDir = () => {},
    onError = () => {},
  } = $props()

  // 編集用のローカルコピー(保存で反映)。task が差し替わったら下の $effect で取り直す。
  // svelte-ignore state_referenced_locally
  let env = $state(cloneEnv(task?.env))
  // svelte-ignore state_referenced_locally
  let note = $state(task?.note || '')
  let dirty = $state(false)
  let saving = $state(false)
  let confirmingDelete = $state(false)
  let confirmingWorkDir = $state(false)
  let noteTimer = null

  // ---- ローカルタスクの編集(概要・説明・優先度・ラベル・リンク・ステータス)。800ms で自動保存
  const isLocal = $derived(sourceOf(task) === 'local')
  // svelte-ignore state_referenced_locally
  let local = $state(cloneLocal(task))
  let localTimer = null
  let labelInput = $state('')
  let linkKey = $state('')
  let linking = $state(false)

  function cloneLocal(t) {
    return {
      summary: t?.jira?.summary || '',
      description: t?.jira?.description || '',
      priority: t?.jira?.priority || 'Medium',
      labels: [...(t?.local?.labels || [])],
      links: [...(t?.local?.links || [])],
      statusId: t?.local?.statusId || '',
    }
  }

  function scheduleLocalSave() {
    clearTimeout(localTimer)
    localTimer = setTimeout(saveLocal, 800)
  }

  async function saveLocal() {
    clearTimeout(localTimer)
    if (!isLocal || !local.summary.trim()) return
    try {
      const updated = await api.patchTask(task.id, {
        local: { ...local, links: local.links.map((l) => l.trim()).filter(Boolean) },
      })
      onChanged(updated)
    } catch (e) {
      onError(e.message)
    }
  }

  function addLabel() {
    const v = labelInput.trim()
    if (v && !local.labels.includes(v)) {
      local.labels = [...local.labels, v]
      scheduleLocalSave()
    }
    labelInput = ''
  }

  async function linkJira() {
    if (!linkKey.trim()) return
    linking = true
    try {
      const updated = await api.linkJira(task.id, linkKey.trim())
      linkKey = ''
      onChanged(updated)
    } catch (e) {
      onError(e.message)
    } finally {
      linking = false
    }
  }

  $effect(() => {
    // task.id が変わったときだけリセット(同じタスクの再取得では編集中の内容を保つ)
    task?.id
    env = cloneEnv(task?.env)
    note = task?.note || ''
    local = cloneLocal(task)
    dirty = false
  })

  function cloneEnv(e) {
    const src = e || {}
    return {
      templateId: src.templateId || '',
      workDir: src.workDir || '',
      branch: src.branch || '',
      layout: src.layout || 'single',
      repos: (src.repos || []).map((r) => ({
        name: r.name,
        source: { kind: r.source?.kind || 'clone', url: r.source?.url || '', baseCloneId: r.source?.baseCloneId || '' },
        setupCommands: [...(r.setupCommands || [])],
      })),
      panes: (src.panes || []).map((p) => ({
        slot: p.slot,
        repoName: p.repoName || '',
        commands: (p.commands || []).map((c) => ({ command: c.command, autoRun: !!c.autoRun })),
      })),
    }
  }

  const editable = $derived(task && task.localState !== 'preparing')
  const envEmpty = $derived(!env.workDir && env.repos.length === 0 && env.panes.length === 0)

  // テンプレートから環境を作り直す(未準備のとき用)
  function applyTemplate(id) {
    const t = templates.find((x) => x.id === id)
    if (!t) return
    const vars = { KEY: task.jiraKey, slug: slugify(task.jira?.summary || '') }
    env = cloneEnv({
      templateId: t.id,
      workDir: expand(t.workDirPattern, vars),
      branch: expand(t.branchPattern, vars),
      layout: t.layout,
      repos: t.repos,
      panes: t.panes,
    })
    dirty = true
  }

  // サーバ(task.Slugify / ExpandPattern)と同じ規則。表示用の先行展開で、保存時はサーバが検証する。
  function slugify(s) {
    let out = ''
    let lastDash = true
    for (const ch of String(s).toLowerCase()) {
      if (/[a-z0-9]/.test(ch)) { out += ch; lastDash = false }
      else if (!lastDash) { out += '-'; lastDash = true }
    }
    out = out.replace(/^-+|-+$/g, '')
    if (out.length > 30) out = out.slice(0, 30).replace(/-+$/, '')
    return out
  }
  function expand(pattern, vars) {
    let out = pattern
    for (const [k, v] of Object.entries(vars)) {
      const ph = `{${k}}`
      if (!v) out = out.split('-' + ph).join('').split('_' + ph).join('').split(ph).join('')
      else out = out.split(ph).join(v)
    }
    return out
  }

  async function saveEnv() {
    saving = true
    try {
      const updated = await api.patchTask(task.id, { env })
      dirty = false
      onChanged(updated)
    } catch (e) {
      onError(e.message)
    } finally {
      saving = false
    }
  }

  // メモは 800ms のデバウンスで自動保存
  function onNoteInput() {
    clearTimeout(noteTimer)
    noteTimer = setTimeout(async () => {
      try {
        const updated = await api.patchTask(task.id, { note })
        onChanged(updated)
      } catch (e) {
        onError(e.message)
      }
    }, 800)
  }

  async function sync() {
    try {
      onChanged(await api.syncTask(task.id))
    } catch (e) {
      onError(e.message)
    }
  }

  async function setDone(done) {
    try {
      onChanged(await api.patchTask(task.id, { localState: done ? 'done' : 'ready' }))
    } catch (e) {
      onError(e.message)
    }
  }

  async function deleteTask() {
    try {
      await api.deleteTask(task.id)
      onDeleted(task)
    } catch (e) {
      onError(e.message)
    }
  }
</script>

{#if task}
  <div class="toolbar">
    <span class="crumb"><button class="link" onclick={onBack}>タスク</button> /</span>
    <span class="src" class:jira={!isLocal} class:local={isLocal}>{isLocal ? '📝 ローカル' : 'Jira'}</span>
    <strong class="key" class:local={isLocal}>{task.jiraKey}</strong>
    <span class="title">{task.jira?.summary}</span>
    {#if isLocal}
      <select class="status" bind:value={local.statusId} onchange={saveLocal}>
        {#each localStatuses as st (st.id)}<option value={st.id}>{st.name}</option>{/each}
      </select>
    {:else}
      <span class="pill">{task.jira?.status || '—'}</span>
    {/if}
    <span class="state"><span class="dot {task.effectiveState}"></span>{TASK_STATE_LABEL[task.effectiveState] || task.effectiveState}</span>
    <span class="spacer"></span>
    {#if isLocal}
      {#if local.links.filter((l) => l.trim()).length > 0}<a class="btn" href={local.links.find((l) => l.trim())} target="_blank" rel="noopener">🔗 リンクを開く ↗</a>{/if}
    {:else}
      {#if task.jira?.url}<a class="btn" href={task.jira.url} target="_blank" rel="noopener">Jira で開く ↗</a>{/if}
      <button onclick={sync} disabled={busy}>⟳ 再取得</button>
    {/if}
    {#if task.effectiveState === 'preparing'}
      <button class="primary" onclick={() => onResume(task)}>⟳ セットアップの進捗</button>
    {:else if task.workspaceName}
      <button class="primary" onclick={() => onOpenWorkspace(task)}>▶ ワークスペースを開く</button>
    {:else if !envEmpty && task.localState !== 'done'}
      <button class="primary" onclick={() => onSetup(task)} disabled={busy || dirty} title={dirty ? '先に環境を保存してください' : ''}>▶ 環境をセットアップ</button>
    {/if}
  </div>

  <div class="body">
    <div class="cols">
      <!-- 左: Jira 情報 + メモ -->
      <div class="col">
        {#if isLocal}
        <div class="card">
          <header><strong>タスク情報</strong><span class="badge">このアプリで管理</span><span class="spacer"></span><span class="muted">自動保存</span></header>
          <div class="inner">
            <dl class="kv form">
              <dt>概要</dt><dd><input bind:value={local.summary} oninput={scheduleLocalSave} /></dd>
              <dt>優先度</dt><dd><select class="auto" bind:value={local.priority} onchange={saveLocal}>{#each ['Highest', 'High', 'Medium', 'Low', 'Lowest'] as p}<option value={p}>{p}</option>{/each}</select></dd>
              <dt>ラベル</dt><dd>
                <div class="chips-in">
                  {#each local.labels as l (l)}
                    <span class="chip">{l}<button class="x" title="削除" onclick={() => { local.labels = local.labels.filter((x) => x !== l); scheduleLocalSave() }}>✕</button></span>
                  {/each}
                  <input class="label-in" bind:value={labelInput} placeholder="追加…" onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addLabel() } }} onblur={addLabel} />
                </div>
              </dd>
              <dt>リンク</dt><dd>
                <div class="links">
                  {#each local.links as _, i}
                    <div class="link-row">
                      <input bind:value={local.links[i]} placeholder="https://…" oninput={scheduleLocalSave} />
                      {#if local.links[i]?.trim()}<a class="icon-link" href={local.links[i]} target="_blank" rel="noopener" title="開く">↗</a>{/if}
                      <button class="icon" title="削除" onclick={() => { local.links = local.links.filter((_, j) => j !== i); scheduleLocalSave() }}>✕</button>
                    </div>
                  {/each}
                  <button class="ghost add" onclick={() => (local.links = [...local.links, ''])}>＋ リンクを追加</button>
                </div>
              </dd>
              <dt class="top">説明</dt><dd><textarea rows="6" bind:value={local.description} oninput={scheduleLocalSave} placeholder="目的・やること・メモ"></textarea></dd>
            </dl>
          </div>
        </div>
        <div class="card">
          <header><strong>Jira に紐付ける</strong><span class="muted">任意</span></header>
          <div class="inner">
            <div class="link-jira">
              <span class="muted">あとで Jira に起票したら:</span>
              <input class="mono" placeholder="PROJ-1310" bind:value={linkKey} onkeydown={(e) => { if (e.key === 'Enter') linkJira() }} />
              <button onclick={linkJira} disabled={linking || !linkKey.trim()}>取得して紐付け</button>
            </div>
            <p class="muted hint">紐付けると番号が Jira の番号に変わり、概要・ステータスは Jira から同期されます（作業フォルダ・ブランチ・ワークスペースはそのまま。ブランチ名は変わりません）。</p>
          </div>
        </div>
        {:else}
        <div class="card">
          <header><strong>Jira 情報</strong><span class="spacer"></span><span class="muted">取得 {relativeTime(task.jira?.fetchedAt)}</span></header>
          <div class="inner">
            <dl class="kv">
              <dt>ステータス</dt><dd>{task.jira?.status || '—'}</dd>
              <dt>担当</dt><dd>{task.jira?.assignee || '未割当'}</dd>
              <dt>優先度</dt><dd>{task.jira?.priority || '—'}</dd>
              {#if task.jira?.epic}<dt>エピック</dt><dd>{task.jira.epic}</dd>{/if}
              {#if task.jira?.sprint}<dt>スプリント</dt><dd>{task.jira.sprint}</dd>{/if}
            </dl>
            {#if task.jira?.description}
              <div class="desc">{task.jira.description}</div>
            {:else}
              <p class="muted">説明はありません</p>
            {/if}
          </div>
        </div>
        {/if}
        <div class="card">
          <header><strong>メモ</strong><span class="muted">ローカルのみ · 自動保存</span></header>
          <div class="inner">
            <textarea rows="5" placeholder="調査メモ、手順、気づきなど" bind:value={note} oninput={onNoteInput}></textarea>
          </div>
        </div>
        <div class="card">
          <header><strong>タスクの状態</strong></header>
          <div class="inner row">
            {#if task.localState === 'done'}
              <button onclick={() => setDone(false)} disabled={busy}>完了を取り消す</button>
            {:else}
              <button onclick={() => setDone(true)} disabled={busy || task.localState === 'preparing'}>✓ 完了にする</button>
            {/if}
            <span class="spacer"></span>
            {#if confirmingWorkDir}
              <span class="muted">作業フォルダとワークスペースを削除します</span>
              <button class="danger" onclick={() => { confirmingWorkDir = false; onDeleteWorkDir(task) }} disabled={busy}>削除する</button>
              <button class="icon" onclick={() => (confirmingWorkDir = false)}>取消</button>
            {:else if task.workDirExists}
              <button onclick={() => (confirmingWorkDir = true)} disabled={busy || task.localState === 'preparing'}>🗑 作業フォルダを削除</button>
            {/if}
            {#if confirmingDelete}
              <span class="muted">タスクだけ削除します（フォルダは残ります）</span>
              <button class="danger" onclick={deleteTask} disabled={busy}>削除する</button>
              <button class="icon" onclick={() => (confirmingDelete = false)}>取消</button>
            {:else}
              <button class="danger" onclick={() => (confirmingDelete = true)} disabled={busy || task.localState === 'preparing'}>タスクを削除</button>
            {/if}
          </div>
        </div>
      </div>

      <!-- 右: 作業環境 -->
      <div class="col">
        <div class="card">
          <header>
            <strong>作業環境</strong>
            {#if env.templateId}
              {@const tpl = templates.find((t) => t.id === env.templateId)}
              <span class="badge">テンプレート: {tpl?.name || '（削除済み）'}</span>
            {/if}
            <span class="spacer"></span>
            {#if editable}
              <select class="tpl-select" value="" onchange={(e) => { applyTemplate(e.currentTarget.value); e.currentTarget.value = '' }} title="テンプレートから環境を作り直す">
                <option value="">テンプレートから展開…</option>
                {#each templates as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
              </select>
            {/if}
          </header>
          <div class="inner">
            {#if !editable}
              <p class="muted">セットアップ実行中は編集できません。</p>
            {/if}
            <fieldset disabled={!editable}>
              <dl class="kv form">
                <dt>作業フォルダ</dt><dd><input class="mono" bind:value={env.workDir} oninput={() => (dirty = true)} placeholder="~/work/PROJ-1301" />{#if task.workDirExists}<span class="ok">✓ 存在します</span>{/if}</dd>
                <dt>ブランチ</dt><dd><input class="mono" bind:value={env.branch} oninput={() => (dirty = true)} placeholder="feature/PROJ-1301-slug（空なら作成しない）" /></dd>
              </dl>
              <div class="sub"><span class="muted">リポジトリとペイン</span></div>
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div oninput={() => (dirty = true)} onchange={() => (dirty = true)}>
                <EnvEditor bind:value={env} {baseClones} />
              </div>
            </fieldset>
            <div class="cta">
              <button class="primary" onclick={saveEnv} disabled={!editable || !dirty || saving}>保存</button>
              {#if dirty}<span class="muted">未保存の変更があります</span>{/if}
              {#if task.workspaceName}
                <span class="spacer"></span>
                <span class="muted">ワークスペース: {task.workspaceName}（{layoutOf(env.layout).label}）</span>
              {:else if task.workspaceMissing}
                <span class="spacer"></span>
                <span class="muted">ワークスペースは削除済み。「環境をセットアップ」で作り直せます</span>
              {/if}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .toolbar .key { font-family: ui-monospace, Menlo, monospace; color: #4c9aff; }
  .toolbar .key.local { color: #c4b5fd; }
  .src { display: inline-flex; align-items: center; gap: 4px; font-size: 10px; border-radius: 8px; padding: 0 6px; border: 1px solid var(--border); white-space: nowrap; }
  .src.jira { color: #4c9aff; border-color: rgba(76, 154, 255, 0.35); }
  .src.local { color: #c4b5fd; border-color: rgba(167, 139, 250, 0.4); background: rgba(167, 139, 250, 0.1); }
  select.status, select.auto { width: auto; padding: 2px 6px; font-size: 12px; }
  .kv dt.top { align-self: start; padding-top: 6px; }
  .chips-in { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; }
  .chip { display: inline-flex; align-items: center; gap: 4px; border: 1px solid var(--border); border-radius: 999px; padding: 1px 8px; font-size: 11px; background: var(--panel-2); }
  .chip .x { background: transparent; border: 0; padding: 0; color: var(--muted); font-size: 10px; }
  .chip .x:hover { color: var(--danger); }
  .label-in { width: 100px; padding: 2px 6px; font-size: 12px; }
  .links { display: flex; flex-direction: column; gap: 4px; width: 100%; }
  .link-row { display: flex; gap: 4px; align-items: center; }
  .link-row input { flex: 1; }
  .icon-link { color: var(--text); text-decoration: none; border: 1px solid var(--border); border-radius: 6px; padding: 2px 6px; font-size: 12px; }
  .add { text-align: left; font-size: 12px; padding: 3px 6px; }
  .link-jira { border: 1px dashed var(--border); border-radius: 8px; padding: 10px 12px; display: flex; gap: 8px; align-items: center; font-size: 12px; flex-wrap: wrap; }
  .link-jira input { width: 150px; }
  .hint { font-size: 11px; margin: 8px 0 0; }
  .toolbar .title { color: #fff; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 40%; }
  .crumb { color: var(--muted); }
  .link { background: transparent; border-color: transparent; padding: 0; color: var(--text); }
  .link:hover:not(:disabled) { color: var(--accent); border-color: transparent; }
  .btn { border: 1px solid var(--border); background: var(--panel-2); color: var(--text); border-radius: 6px; padding: 6px 10px; font-size: 13px; text-decoration: none; }
  .btn:hover { border-color: var(--accent); }
  .pill { font-size: 11px; border-radius: 4px; padding: 1px 6px; border: 1px solid var(--border); white-space: nowrap; }
  .state { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
  .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; background: var(--muted); }
  .dot.working { background: #22c55e; }
  .dot.preparing { background: #f59e0b; }
  .dot.ready { background: var(--accent); }
  .dot.none { background: transparent; border: 1.5px solid var(--muted); }
  .body { padding: 16px; overflow-y: auto; flex: 1; min-height: 0; }
  .cols { display: grid; grid-template-columns: minmax(280px, 1fr) minmax(360px, 1.4fr); gap: 16px; align-items: start; }
  .col { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
  .card { border: 1px solid var(--border); border-radius: 8px; background: var(--panel); }
  .card > header { display: flex; align-items: center; gap: 8px; padding: 10px 14px; border-bottom: 1px solid var(--border); }
  .card > header strong { color: #fff; font-size: 13px; }
  .inner { padding: 12px 14px; }
  .inner.row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .kv { display: grid; grid-template-columns: 110px 1fr; gap: 6px 10px; margin: 0 0 10px; font-size: 13px; }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; min-width: 0; }
  .kv.form dd { display: flex; align-items: center; gap: 8px; }
  .desc { white-space: pre-wrap; font-size: 12.5px; line-height: 1.55; border-top: 1px solid var(--border); padding-top: 10px; max-height: 320px; overflow-y: auto; }
  .badge { font-size: 10px; color: var(--muted); border: 1px solid var(--border); border-radius: 8px; padding: 0 6px; }
  .tpl-select { width: auto; font-size: 12px; padding: 3px 8px; }
  .mono { font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
  .ok { color: #22c55e; font-size: 11px; white-space: nowrap; }
  .sub { margin: 12px 0 6px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
  .cta { display: flex; align-items: center; gap: 10px; margin-top: 12px; flex-wrap: wrap; }
  textarea { width: 100%; }
</style>
