<script>
  // タスクを追加するモーダル。「Jira から取り込む」と「手動で作成（ローカルタスク）」のタブ。
  // Jira: 番号入力 → 取得(プレビュー) → テンプレート選択 → 取り込み。
  // 手動: 番号(自動採番)・概要・説明・優先度・ラベル・リンク・ステータス → 作成。
  import { onMount, tick } from 'svelte'
  import { api, layoutOf } from '../api.js'
  import { parseJiraKeys } from '../tasks.js'

  let {
    templates = [],
    jiraReady = true, // Jira 接続が設定済みか。未設定なら手動タブだけ
    localStatuses = [],
    initialTab = 'jira',
    onClose = () => {},
    onImported = () => {},
    onCreated = () => {},
    onError = () => {},
  } = $props()

  // svelte-ignore state_referenced_locally
  let tab = $state(jiraReady ? initialTab : 'local')
  let templateId = $state('')
  let busy = $state(false)
  let inputEl = $state()

  // ---- Jira タブ
  let text = $state('')
  let items = $state([]) // PreviewItem[]
  let previewed = $state(false)
  const keys = $derived(parseJiraKeys(text))
  const template = $derived(templates.find((t) => t.id === templateId) || null)
  const importable = $derived(items.filter((i) => !i.exists))

  // ---- 手動タブ
  let nextKey = $state('')
  let prefix = $state('T')
  let seqText = $state('')
  let summary = $state('')
  let description = $state('')
  let priority = $state('Medium')
  let statusId = $state('')
  let labels = $state([])
  let labelInput = $state('')
  let links = $state([''])
  let summaryEl = $state()

  // 番号の入力欄は「接頭辞-」を固定表示し、連番だけ編集する
  const localKey = $derived(seqText.trim() ? `${prefix}-${seqText.trim()}` : nextKey)
  const localSlug = $derived(slugify(summary))

  onMount(async () => {
    const def = templates.find((t) => t.isDefault) || templates[0]
    if (def) templateId = def.id
    if (localStatuses.length) statusId = localStatuses[0].id
    try {
      const r = await api.nextLocalKey()
      nextKey = r?.key || ''
      const i = nextKey.lastIndexOf('-')
      if (i > 0) {
        prefix = nextKey.slice(0, i)
        seqText = nextKey.slice(i + 1)
      }
    } catch {
      // 設定が無いなど。空のまま(サーバが採番する)
    }
    await tick()
    focusFirst()
  })

  function focusFirst() {
    if (tab === 'jira') inputEl?.focus()
    else summaryEl?.focus()
  }

  function switchTab(t) {
    tab = t
    tick().then(focusFirst)
  }

  // サーバ(task.Slugify / ExpandPattern)と同じ規則(表示用)
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

  async function preview() {
    if (keys.length === 0) return
    busy = true
    try {
      const res = await api.previewTasks(keys, templateId)
      items = res?.items || []
      previewed = true
    } catch (e) {
      onError(e.message)
    } finally {
      busy = false
    }
  }

  async function onTemplateChange() {
    if (tab === 'jira' && previewed) await preview()
  }

  async function doImport(setup) {
    const ks = importable.map((i) => i.issue.key)
    if (ks.length === 0) return
    busy = true
    try {
      const res = await api.importTasks(ks, templateId, setup)
      onImported(res, setup)
    } catch (e) {
      onError(e.message)
    } finally {
      busy = false
    }
  }

  function addLabel() {
    const v = labelInput.trim()
    if (v && !labels.includes(v)) labels = [...labels, v]
    labelInput = ''
  }

  async function doCreate(setup) {
    if (!summary.trim()) {
      onError('概要を入力してください')
      summaryEl?.focus()
      return
    }
    busy = true
    try {
      const res = await api.createLocalTask({
        key: seqText.trim() ? localKey : '',
        summary: summary.trim(),
        description,
        priority,
        labels,
        links: links.map((l) => l.trim()).filter(Boolean),
        statusId,
        templateId: templateId || '',
        setup,
      })
      onCreated(res, setup)
    } catch (e) {
      onError(e.message)
    } finally {
      busy = false
    }
  }

  function onKey(e) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
    } else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      if (tab === 'jira') preview()
      else doCreate(!!template)
    }
  }

  function repoLine(r) {
    return `${r.name} ${r.source?.kind === 'baseCopy' ? '⧉' : '⤓'}`
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="overlay" role="presentation" onclick={onClose}>
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="modal" role="dialog" aria-labelledby="imp-title" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={onKey}>
    <header>
      <strong id="imp-title">タスクを追加</strong>
      <span class="spacer"></span>
      <button class="icon" title="閉じる" onclick={onClose}>✕</button>
    </header>
    <div class="tabs" role="tablist">
      <button role="tab" aria-selected={tab === 'jira'} class:on={tab === 'jira'} disabled={!jiraReady} title={jiraReady ? '' : 'Jira 接続が未設定です（⚙ 設定 → Jira 接続）'} onclick={() => switchTab('jira')}>🔗 Jira から取り込む</button>
      <button role="tab" aria-selected={tab === 'local'} class:on={tab === 'local'} onclick={() => switchTab('local')}>📝 手動で作成</button>
    </div>

    <div class="content">
      {#if tab === 'jira'}
        <label class="field">
          <span>Jira 番号</span>
          <div class="inline">
            <input bind:this={inputEl} placeholder="PROJ-1301, PROJ-1302 または URL" bind:value={text} />
            <button onclick={preview} disabled={busy || keys.length === 0}>取得</button>
          </div>
          <span class="hint muted">カンマ区切りで複数指定できます。Jira の URL を貼り付けても番号を拾います。{#if keys.length}　→ {keys.join(', ')}{/if}</span>
        </label>

        {#if previewed}
          {#if items.length === 0}
            <p class="muted">該当する issue がありません</p>
          {:else}
            <ul class="issues">
              {#each items as it (it.issue.key)}
                <li class:exists={it.exists}>
                  <span class="key">{it.issue.key}</span>
                  <span class="sum">{it.issue.summary}</span>
                  <span class="pill">{it.issue.status}</span>
                  {#if it.exists}<span class="warn">取り込み済み</span>{/if}
                  <dl>
                    <dt>担当</dt><dd>{it.issue.assignee || '未割当'}</dd>
                    <dt>優先度</dt><dd>{it.issue.priority || '—'}</dd>
                    {#if it.issue.epic}<dt>エピック</dt><dd>{it.issue.epic}</dd>{/if}
                    {#if it.issue.sprint}<dt>スプリント</dt><dd>{it.issue.sprint}</dd>{/if}
                  </dl>
                  {#if it.issue.description}<p class="desc muted">{it.issue.description.slice(0, 160)}{it.issue.description.length > 160 ? '…' : ''}</p>{/if}
                </li>
              {/each}
            </ul>
          {/if}

          <label class="field">
            <span>環境テンプレート</span>
            <select bind:value={templateId} onchange={onTemplateChange}>
              {#each templates as t (t.id)}
                <option value={t.id}>{t.name}（{t.repos.map(repoLine).join(' + ') || 'リポジトリなし'} · {layoutOf(t.layout).label}）</option>
              {/each}
              <option value="">なし（後でタスク詳細から設定）</option>
            </select>
          </label>

          {#if template && importable.length > 0}
            {@const first = importable[0]}
            <div class="box">
              <h4>作成されるもの{#if importable.length > 1}<span class="muted">（{first.issue.key} の例。他 {importable.length - 1} 件も同様）</span>{/if}</h4>
              {#if first.env}
                <dl class="kv">
                  <dt>作業フォルダ</dt><dd><code>{first.env.workDir}</code></dd>
                  {#each first.env.repos as r (r.name)}
                    <dt></dt><dd><code>{r.name}</code> ← {r.source.kind === 'baseCopy' ? 'ベースクローンをコピー' : `git clone ${r.source.url}`}</dd>
                  {/each}
                  <dt>ブランチ</dt><dd><code>{first.env.branch || '（作成しない）'}</code></dd>
                  <dt>ワークスペース</dt><dd>{first.issue.key} {first.issue.summary}<br /><span class="muted">{layoutOf(first.env.layout).label} · {first.env.panes.map((p) => p.repoName || 'ルート').join(' / ') || 'ペインなし'}</span></dd>
                </dl>
              {/if}
            </div>
          {/if}
        {/if}
      {:else}
        <div class="callout">Jira に無い作業（調査・環境整備・個人的な TODO など）も同じ一覧・同じ環境セットアップで扱えます。<b>あとから Jira 番号を紐付けて Jira タスクに切り替える</b>こともできます。</div>
        <div class="form2">
          <label class="field">
            <span>番号</span>
            <div class="keygen"><span class="pfx">{prefix}-</span><input class="seq" bind:value={seqText} placeholder="自動" /><span class="muted hint">自動採番（接頭辞は設定で変更）</span></div>
          </label>
          <label class="field">
            <span>ステータス</span>
            <select bind:value={statusId}>
              {#each localStatuses as st (st.id)}<option value={st.id}>{st.name}</option>{/each}
            </select>
          </label>
          <label class="field full">
            <span>概要 <em>*</em></span>
            <input bind:this={summaryEl} bind:value={summary} placeholder="例: Playwright で E2E テストの雛形を作る" />
          </label>
          <label class="field full">
            <span>説明</span>
            <textarea rows="3" bind:value={description} placeholder="目的・やること・メモ"></textarea>
          </label>
          <label class="field">
            <span>優先度</span>
            <select bind:value={priority}>
              {#each ['Highest', 'High', 'Medium', 'Low', 'Lowest'] as p}<option value={p}>{p}</option>{/each}
            </select>
          </label>
          <div class="field">
            <span>ラベル</span>
            <div class="chips-in">
              {#each labels as l (l)}
                <span class="chip">{l}<button class="x" title="削除" onclick={() => (labels = labels.filter((x) => x !== l))}>✕</button></span>
              {/each}
              <input class="label-in" bind:value={labelInput} placeholder="追加…" onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addLabel() } }} onblur={addLabel} />
            </div>
          </div>
          <div class="field full">
            <span>参考リンク（任意・複数可）</span>
            {#each links as _, i}
              <div class="inline">
                <input bind:value={links[i]} placeholder="https://…" />
                {#if links.length > 1}<button class="icon" title="削除" onclick={() => (links = links.filter((_, j) => j !== i))}>✕</button>{/if}
              </div>
            {/each}
            <button class="ghost add" onclick={() => (links = [...links, ''])}>＋ リンクを追加</button>
          </div>
          <label class="field full">
            <span>環境テンプレート</span>
            <select bind:value={templateId}>
              {#each templates as t (t.id)}
                <option value={t.id}>{t.name}（{t.repos.map(repoLine).join(' + ') || 'リポジトリなし'} · {layoutOf(t.layout).label}）</option>
              {/each}
              <option value="">なし（後でタスク詳細から設定）</option>
            </select>
          </label>
        </div>
        {#if template}
          <div class="box">
            <h4>作成されるもの</h4>
            <dl class="kv">
              <dt>作業フォルダ</dt><dd><code>{expand(template.workDirPattern, { KEY: localKey, slug: localSlug })}</code></dd>
              <dt>ブランチ</dt><dd><code>{expand(template.branchPattern, { KEY: localKey, slug: localSlug })}</code> <span class="muted">（{'{KEY}'} = {localKey}、{'{slug}'} は概要の ASCII 部分）</span></dd>
              <dt>ワークスペース</dt><dd>{localKey} {summary || '（概要）'} <span class="muted">· {layoutOf(template.layout).label}</span></dd>
            </dl>
          </div>
        {/if}
      {/if}
    </div>

    <footer>
      <span class="muted">{tab === 'jira' ? 'Jira 側は変更しません' : 'Jira には何も作成しません'}</span>
      <span class="spacer"></span>
      <button onclick={onClose} disabled={busy}>キャンセル</button>
      {#if tab === 'jira'}
        <button onclick={() => doImport(false)} disabled={busy || importable.length === 0}>取り込む（環境は後で）</button>
        <button class="primary" onclick={() => doImport(true)} disabled={busy || importable.length === 0 || !template}>取り込んでセットアップ</button>
      {:else}
        <button onclick={() => doCreate(false)} disabled={busy || !summary.trim()}>作成（環境は後で）</button>
        <button class="primary" onclick={() => doCreate(true)} disabled={busy || !summary.trim() || !template}>作成してセットアップ</button>
      {/if}
    </footer>
  </div>
</div>

<style>
  .overlay { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.55); display: flex; align-items: center; justify-content: center; z-index: 100; }
  .modal { background: var(--panel); border: 1px solid var(--border); border-radius: 10px; width: min(720px, calc(100vw - 40px)); max-height: calc(100vh - 60px); display: flex; flex-direction: column; box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5); }
  header, footer { display: flex; align-items: center; gap: 10px; padding: 14px 18px; }
  header { border-bottom: 0; padding-bottom: 6px; }
  header strong { color: #fff; font-size: 14px; }
  footer { border-top: 1px solid var(--border); }
  .tabs { display: flex; border-bottom: 1px solid var(--border); padding: 0 18px; }
  .tabs button { background: transparent; border: 0; border-bottom: 2px solid transparent; border-radius: 0; padding: 8px 12px; color: var(--muted); }
  .tabs button.on { color: #fff; border-bottom-color: var(--accent); }
  .tabs button:hover:not(:disabled) { border-color: transparent; border-bottom-color: var(--border); }
  .tabs button:disabled { opacity: 0.5; }
  .content { padding: 16px 18px; overflow-y: auto; display: flex; flex-direction: column; gap: 14px; }
  .field { display: flex; flex-direction: column; gap: 6px; font-size: 12px; }
  .field > span { color: var(--muted); }
  .field em { color: var(--danger); font-style: normal; }
  .inline { display: flex; gap: 6px; }
  .inline input { flex: 1; }
  .hint { font-size: 11px; }
  .form2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px 16px; }
  .form2 .full { grid-column: 1 / -1; }
  .callout { border: 1px solid rgba(167, 139, 250, 0.4); background: rgba(167, 139, 250, 0.08); border-radius: 8px; padding: 10px 12px; font-size: 12px; }
  .callout b { color: #c4b5fd; }
  .keygen { display: flex; gap: 6px; align-items: center; }
  .keygen .pfx { font-family: ui-monospace, Menlo, monospace; color: #c4b5fd; }
  .keygen .seq { width: 90px; font-family: ui-monospace, Menlo, monospace; }
  .chips-in { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; }
  .chip { display: inline-flex; align-items: center; gap: 4px; border: 1px solid var(--border); border-radius: 999px; padding: 1px 8px; font-size: 11px; background: var(--panel-2); }
  .chip .x { background: transparent; border: 0; padding: 0; color: var(--muted); font-size: 10px; }
  .chip .x:hover { color: var(--danger); }
  .label-in { width: 110px; padding: 2px 6px; font-size: 12px; }
  .add { text-align: left; font-size: 12px; padding: 4px 6px; }
  .issues { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .issues li { border: 1px solid var(--border); border-radius: 8px; padding: 10px 12px; background: var(--panel-2); display: grid; grid-template-columns: auto 1fr auto auto; gap: 4px 10px; align-items: center; }
  .issues li.exists { opacity: 0.6; }
  .issues .key { font-family: ui-monospace, Menlo, monospace; color: #4c9aff; font-size: 12px; }
  .issues .sum { color: #fff; }
  .pill { font-size: 11px; border-radius: 4px; padding: 1px 6px; border: 1px solid var(--border); white-space: nowrap; }
  .warn { font-size: 11px; color: #f59e0b; white-space: nowrap; }
  .issues dl { grid-column: 1 / -1; display: grid; grid-template-columns: auto 1fr auto 1fr; gap: 2px 10px; margin: 4px 0 0; font-size: 12px; }
  .issues dt { color: var(--muted); }
  .issues dd { margin: 0; }
  .desc { grid-column: 1 / -1; margin: 4px 0 0; font-size: 12px; white-space: pre-wrap; }
  .box { border: 1px solid var(--border); border-radius: 8px; padding: 10px 14px; background: var(--panel-2); }
  .box h4 { margin: 0 0 8px; font-size: 12px; color: #fff; }
  .box h4 .muted { font-weight: 400; margin-left: 6px; }
  .kv { display: grid; grid-template-columns: 110px 1fr; gap: 4px 10px; margin: 0; font-size: 12px; }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; }
  code { font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
</style>
