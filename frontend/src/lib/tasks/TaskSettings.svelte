<script>
  // タスク環境の設定画面（環境テンプレート / ベースクローン / Jira 接続 / セットアップ設定）。
  // 右上の ⚙ メニューから開く。左のサブナビで panel を切り替える（切替は親に通知）。
  import { onMount } from 'svelte'
  import { api, layoutOf } from '../api.js'
  import { formatBytes, relativeTime } from '../tasks.js'
  import EnvEditor from './EnvEditor.svelte'

  let { panel = 'templates', onNavigate = () => {}, onError = () => {} } = $props()

  const NAV = [
    { grp: 'タスク環境' },
    { id: 'templates', label: '🧩 環境テンプレート' },
    { id: 'base', label: '⧉ ベースクローン' },
    { id: 'jira', label: '🔗 Jira 接続' },
    { id: 'local', label: '📝 ローカルタスク' },
    { grp: '全体' },
    { id: 'general', label: '⚙ セットアップ設定' },
  ]
  const TITLES = { templates: '環境テンプレート', base: 'ベースクローン', jira: 'Jira 接続', local: 'ローカルタスク', general: 'セットアップ設定' }
  const CATEGORIES = [
    { value: 'new', label: '未着手' },
    { value: 'indeterminate', label: '進行中' },
    { value: 'done', label: '完了' },
  ]

  let busy = $state(false)
  let templates = $state([])
  let baseClones = $state([])
  let settings = $state({
    autoFetchBeforeSetup: true,
    includeNodeModules: true,
    localTask: { prefix: 'T', digits: 3, nextSeq: 1, statuses: [] },
  })
  let jira = $state({ kind: 'cloud', baseUrl: '', email: '', jql: '', hasToken: false, token: '', clearToken: false })
  let jiraTest = $state(null) // {ok, displayName, email, error}

  // テンプレート編集中の下書き（null = 未選択）。保存するまでサーバへは送らない
  let draft = $state(null)
  let draftId = $state('') // '' = 新規
  let confirmingTplDelete = $state(false)

  // ベースクローン追加フォーム
  let bcName = $state('')
  let bcUrl = $state('')
  let bcPath = $state('')
  let cloning = $state(false)
  let confirmingBcId = $state(null)
  let removeBcFiles = $state(false)
  let bcErrors = $state([])
  let savedMsg = $state('')

  async function guard(fn) {
    busy = true
    try {
      await fn()
    } catch (e) {
      onError(e.message || String(e))
    } finally {
      busy = false
    }
  }

  function flash(msg) {
    savedMsg = msg
    setTimeout(() => (savedMsg = ''), 2000)
  }

  async function loadAll() {
    await guard(async () => {
      const [t, b, s, j] = await Promise.all([
        api.listTemplates(),
        api.listBaseClones(),
        api.getTaskSettings(),
        api.getJiraConfig(),
      ])
      templates = t || []
      baseClones = b || []
      settings = { ...settings, ...(s || {}) }
      if (!settings.localTask) settings.localTask = { prefix: 'T', digits: 3, nextSeq: 1, statuses: [] }
      jira = { ...jira, ...(j || {}), token: '', clearToken: false }
      if (!draft && templates.length > 0) selectTemplate(templates[0])
    })
  }
  onMount(loadAll)

  // ---- テンプレート ----
  function emptyTemplate() {
    return {
      name: '',
      isDefault: templates.length === 0,
      workDirPattern: '~/work/{KEY}',
      branchPattern: 'feature/{KEY}-{slug}',
      layout: 'single',
      repos: [],
      panes: [{ slot: 0, repoName: '', commands: [] }],
    }
  }

  function selectTemplate(t) {
    draftId = t.id
    // 深いコピーで編集する（一覧側の表示を保存前に変えない）
    draft = JSON.parse(JSON.stringify({
      name: t.name,
      isDefault: t.isDefault,
      workDirPattern: t.workDirPattern,
      branchPattern: t.branchPattern,
      layout: t.layout,
      repos: t.repos || [],
      panes: t.panes || [],
    }))
    confirmingTplDelete = false
  }

  function newTemplate() {
    draftId = ''
    draft = emptyTemplate()
    confirmingTplDelete = false
  }

  function templateSummary(t) {
    const repos = (t.repos || []).map((r) => `${r.name} ${r.source?.kind === 'baseCopy' ? '⧉' : '⤓'}`).join(' + ')
    return `${repos || 'リポジトリなし'} · ${layoutOf(t.layout).label}`
  }

  function saveTemplate() {
    if (!draft) return
    const input = JSON.parse(JSON.stringify(draft))
    guard(async () => {
      const saved = draftId ? await api.updateTemplate(draftId, input) : await api.createTemplate(input)
      templates = (await api.listTemplates()) || []
      const cur = templates.find((t) => t.id === saved.id)
      if (cur) selectTemplate(cur)
      flash('保存しました')
    })
  }

  function deleteTemplate() {
    if (!draftId) return
    guard(async () => {
      await api.deleteTemplate(draftId)
      templates = (await api.listTemplates()) || []
      draft = null
      draftId = ''
      confirmingTplDelete = false
      if (templates.length > 0) selectTemplate(templates[0])
    })
  }

  // ---- ベースクローン ----
  function fetchAge(iso) {
    const t = Date.parse(iso)
    if (!iso || Number.isNaN(t)) return 'stale'
    return Date.now() - t > 3 * 24 * 3600 * 1000 ? 'stale' : 'fresh'
  }

  function addBaseClone() {
    if (!bcName.trim() || !bcUrl.trim()) {
      onError('リポジトリ名と URL を入力してください')
      return
    }
    cloning = true
    guard(async () => {
      await api.createBaseClone({ name: bcName.trim(), url: bcUrl.trim(), path: bcPath.trim() })
      baseClones = (await api.listBaseClones()) || []
      bcName = ''
      bcUrl = ''
      bcPath = ''
    }).finally(() => (cloning = false))
  }

  function updateBaseClone(id) {
    guard(async () => {
      await api.updateBaseClone(id)
      baseClones = (await api.listBaseClones()) || []
    })
  }

  function updateAllBaseClones() {
    guard(async () => {
      const res = await api.updateAllBaseClones()
      baseClones = res?.baseClones || (await api.listBaseClones()) || []
      bcErrors = res?.errors || []
    })
  }

  function deleteBaseClone(id) {
    guard(async () => {
      await api.deleteBaseClone(id, removeBcFiles)
      baseClones = (await api.listBaseClones()) || []
      confirmingBcId = null
      removeBcFiles = false
    })
  }

  // ---- Jira ----
  function saveJira() {
    guard(async () => {
      const res = await api.putJiraConfig({
        kind: jira.kind,
        baseUrl: jira.baseUrl,
        email: jira.email,
        jql: jira.jql,
        token: jira.token,
        clearToken: jira.clearToken,
      })
      jira = { ...jira, ...(res || {}), token: '', clearToken: false }
      jiraTest = null
      flash('保存しました')
    })
  }

  function testJira() {
    guard(async () => {
      jiraTest = await api.testJira()
    })
  }

  // ---- ローカルタスク設定 ----
  function pad(n) {
    return String(n).padStart(Number(settings.localTask?.digits) || 1, '0')
  }
  function addStatus() {
    settings.localTask.statuses = [...settings.localTask.statuses, { id: '', name: '', category: 'new' }]
  }
  function removeStatus(i) {
    settings.localTask.statuses = settings.localTask.statuses.filter((_, j) => j !== i)
  }
  function moveStatus(i, d) {
    const arr = [...settings.localTask.statuses]
    const j = i + d
    if (j < 0 || j >= arr.length) return
    ;[arr[i], arr[j]] = [arr[j], arr[i]]
    settings.localTask.statuses = arr
  }

  // ---- 全体設定 ----
  function saveSettings() {
    guard(async () => {
      settings = (await api.putTaskSettings({ ...settings })) || settings
      flash('保存しました')
    })
  }
</script>

<div class="settings-wrap">
  <nav class="settings-nav" aria-label="設定の項目">
    {#each NAV as n}
      {#if n.grp}
        <div class="grp">{n.grp}</div>
      {:else}
        <button class:on={panel === n.id} onclick={() => onNavigate(n.id)}>{n.label}</button>
      {/if}
    {/each}
  </nav>

  <div class="settings-panel">
    <div class="panel-head">
      <strong>{TITLES[panel] || '設定'}</strong>
      {#if savedMsg}<span class="ok">✓ {savedMsg}</span>{/if}
    </div>

    {#if panel === 'templates'}
      <div class="two-col">
        <div class="card">
          <header>
            <strong>テンプレート</strong>
            <span class="spacer"></span>
            <button class="icon" onclick={newTemplate} disabled={busy}>＋ 新規テンプレート</button>
          </header>
          <div class="inner tpl">
            {#if templates.length === 0}
              <p class="muted">まだありません</p>
            {/if}
            {#each templates as t (t.id)}
              <button class="tpl-row" class:sel={draftId === t.id} onclick={() => selectTemplate(t)}>
                <span class="n">{t.name}</span>
                {#if t.isDefault}<span class="muted">既定</span>{/if}
                <span class="d">{templateSummary(t)}</span>
              </button>
            {/each}
          </div>
        </div>

        <div class="card">
          <header><strong>{draftId ? '編集' : '新規作成'}</strong></header>
          <div class="inner">
            {#if !draft}
              <p class="muted">左の一覧からテンプレートを選ぶか、「＋ 新規テンプレート」を押してください。</p>
            {:else}
              <dl class="kv">
                <dt>名前</dt>
                <dd><input bind:value={draft.name} placeholder="web-app 標準" /></dd>
                <dt>既定</dt>
                <dd>
                  <label class="inline-check">
                    <input type="checkbox" bind:checked={draft.isDefault} />
                    取り込み時に最初に選ばれるテンプレートにする
                  </label>
                </dd>
                <dt>作業フォルダ</dt>
                <dd><input class="mono" bind:value={draft.workDirPattern} placeholder="~/work/{'{KEY}'}" /></dd>
                <dt>ブランチ名</dt>
                <dd><input class="mono" bind:value={draft.branchPattern} placeholder="feature/{'{KEY}'}-{'{slug}'}" /></dd>
              </dl>
              <p class="muted hint">{'{KEY}'} = Jira 番号、{'{slug}'} = 概要から生成（ASCII のみ。空なら省略）</p>
              <EnvEditor bind:value={draft} {baseClones} />
              <div class="cta">
                <button class="primary" onclick={saveTemplate} disabled={busy}>保存</button>
                {#if draftId}
                  {#if confirmingTplDelete}
                    <span class="muted">このテンプレートを削除しますか？（取り込み済みタスクには影響しません）</span>
                    <button class="danger" onclick={deleteTemplate} disabled={busy}>削除する</button>
                    <button class="icon" onclick={() => (confirmingTplDelete = false)}>取消</button>
                  {:else}
                    <button class="danger" onclick={() => (confirmingTplDelete = true)} disabled={busy}>削除</button>
                  {/if}
                {/if}
              </div>
            {/if}
          </div>
        </div>
      </div>

    {:else if panel === 'base'}
      <div class="card">
        <header>
          <strong>ベースクローン</strong>
          <span class="muted">コピー元として保持する clone</span>
          <span class="spacer"></span>
          <button class="icon" onclick={updateAllBaseClones} disabled={busy || baseClones.length === 0}>⟳ すべて更新</button>
        </header>
        <div class="inner" style="padding:0;">
          {#if baseClones.length === 0}
            <p class="muted" style="padding: 12px 14px; margin: 0;">まだありません。下のフォームから clone を登録してください。</p>
          {:else}
            <div class="table-wrap">
              <table class="base-table">
                <thead>
                  <tr><th>リポジトリ</th><th>パス</th><th>既定ブランチ</th><th>最終 fetch</th><th>サイズ</th><th></th></tr>
                </thead>
                <tbody>
                  {#each baseClones as bc (bc.id)}
                    <tr>
                      <td class="mono">{bc.name}</td>
                      <td class="mono muted" title={bc.path}>{bc.path}</td>
                      <td class="mono">{bc.defaultBranch || '—'}</td>
                      <td class={fetchAge(bc.lastFetchedAt)}>{relativeTime(bc.lastFetchedAt) || '未取得'}</td>
                      <td class="num">{formatBytes(bc.sizeBytes)}</td>
                      <td class="actions">
                        {#if confirmingBcId === bc.id}
                          <label class="inline-check"><input type="checkbox" bind:checked={removeBcFiles} /> フォルダも削除</label>
                          <button class="icon danger" onclick={() => deleteBaseClone(bc.id)} disabled={busy}>削除？</button>
                          <button class="icon" onclick={() => (confirmingBcId = null)}>取消</button>
                        {:else}
                          <button class="icon" onclick={() => updateBaseClone(bc.id)} disabled={busy}>⟳ 更新</button>
                          <button class="icon danger" title="削除" onclick={() => { confirmingBcId = bc.id; removeBcFiles = false }}>✕</button>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
          {#if bcErrors.length > 0}
            <ul class="errs">
              {#each bcErrors as e}<li>{e}</li>{/each}
            </ul>
          {/if}
          <div class="add-bc">
            <strong>＋ 追加</strong>
            <div class="add-bc-row">
              <input placeholder="名前（例: frontend）" bind:value={bcName} disabled={cloning} />
              <input class="mono" placeholder="git@github.com:org/repo.git" bind:value={bcUrl} disabled={cloning} />
              <input class="mono" placeholder="空なら MULTI_TERMINALS_DIR/base/<name>" bind:value={bcPath} disabled={cloning} />
              <button class="primary" onclick={addBaseClone} disabled={busy || cloning}>{cloning ? 'clone 中…' : 'clone して登録'}</button>
            </div>
            <p class="muted hint">clone は同期で行います。大きなリポジトリでは時間がかかります。</p>
          </div>
        </div>
      </div>

    {:else if panel === 'jira'}
      <div class="card narrow">
        <header>
          <strong>Jira 接続</strong>
          <span class="spacer"></span>
          {#if jiraTest?.ok}
            <span class="fresh">✓ 接続済み: {jiraTest.displayName}{jiraTest.email ? ` (${jiraTest.email})` : ''}</span>
          {:else if jiraTest}
            <span class="bad">✗ {jiraTest.error}</span>
          {:else if jira.hasToken}
            <span class="muted">トークン設定済み</span>
          {/if}
        </header>
        <div class="inner">
          <dl class="kv">
            <dt>種類</dt>
            <dd>
              <select bind:value={jira.kind}>
                <option value="cloud">Jira Cloud（メール + API トークン）</option>
                <option value="server">Jira Server / Data Center（Personal Access Token）</option>
              </select>
            </dd>
            <dt>サイト URL</dt>
            <dd><input bind:value={jira.baseUrl} placeholder={jira.kind === 'cloud' ? 'https://acme.atlassian.net' : 'https://jira.example.com'} /></dd>
            {#if jira.kind === 'cloud'}
              <dt>メール</dt>
              <dd><input bind:value={jira.email} placeholder="you@example.com" /></dd>
            {/if}
            <dt>{jira.kind === 'cloud' ? 'API トークン' : 'アクセストークン'}</dt>
            <dd>
              <div class="inline">
                <input
                  type="password"
                  bind:value={jira.token}
                  placeholder={jira.hasToken && !jira.clearToken ? '設定済み（変更する場合のみ入力）' : 'トークンを入力'}
                  autocomplete="off"
                />
                {#if jira.hasToken && !jira.clearToken}
                  <button class="icon danger" style="white-space: nowrap" onclick={() => (jira.clearToken = true)}>トークンを削除</button>
                {:else if jira.clearToken}
                  <span class="muted">保存時に削除</span>
                  <button class="icon" onclick={() => (jira.clearToken = false)}>取消</button>
                {/if}
              </div>
            </dd>
            <dt>自動取り込み JQL</dt>
            <dd>
              <textarea rows="2" class="mono" bind:value={jira.jql} placeholder="assignee = currentUser() AND sprint in openSprints()"></textarea>
            </dd>
          </dl>
          <p class="muted hint">
            トークンは MULTI_TERMINALS_DIR 配下に 0600 で保存し、設定 JSON には書きません。JQL を設定すると「Jira を同期」で該当タスクを未準備として自動追加します。
          </p>
          <div class="cta">
            <button class="primary" onclick={saveJira} disabled={busy}>保存</button>
            <button onclick={testJira} disabled={busy}>接続テスト</button>
          </div>
        </div>
      </div>

    {:else if panel === 'local'}
      <div class="card narrow">
        <header><strong>ローカルタスク</strong><span class="muted">Jira を使わないタスクの番号とステータス</span></header>
        <div class="inner">
          <dl class="kv">
            <dt>番号の接頭辞</dt>
            <dd>
              <div class="row">
                <input class="mono short" bind:value={settings.localTask.prefix} placeholder="T" />
                <span class="muted">次は <code>{settings.localTask.prefix || 'T'}-{pad(settings.localTask.nextSeq || 1)}</code>。Jira のプロジェクトキーと被らないものにしてください</span>
              </div>
            </dd>
            <dt>桁数</dt>
            <dd><div class="row"><input type="number" min="1" max="6" class="short" bind:value={settings.localTask.digits} /><span class="muted">{settings.localTask.prefix || 'T'}-{pad(8)} のようにゼロ埋め</span></div></dd>
          </dl>
          <div class="sub">ステータス（並び順 = 表示順）</div>
          <div class="status-list">
            {#each settings.localTask.statuses as st, i (i)}
              <div class="status-row">
                <span class="order">
                  <button class="icon" title="上へ" disabled={i === 0} onclick={() => moveStatus(i, -1)}>▲</button>
                  <button class="icon" title="下へ" disabled={i === settings.localTask.statuses.length - 1} onclick={() => moveStatus(i, 1)}>▼</button>
                </span>
                <input bind:value={st.name} placeholder="ステータス名" />
                <select bind:value={st.category}>
                  {#each CATEGORIES as c}<option value={c.value}>{c.label}</option>{/each}
                </select>
                <button class="icon danger" title="削除" disabled={settings.localTask.statuses.length <= 1} onclick={() => removeStatus(i)}>✕</button>
              </div>
            {/each}
            <button class="ghost add" onclick={addStatus}>＋ ステータスを追加</button>
          </div>
          <p class="muted note">「完了」カテゴリのステータスにしてもローカル状態（環境あり / 完了）は自動では変わりません。Jira タスクと同じく「完了にする」は手動です。削除したステータスを使っていたタスクは先頭のステータスとして表示されます。</p>
          <div class="cta">
            <button class="primary" onclick={saveSettings} disabled={busy}>保存</button>
          </div>
        </div>
      </div>
    {:else if panel === 'general'}
      <div class="card narrow">
        <header><strong>セットアップ設定</strong></header>
        <div class="inner">
          <label class="check"><input type="checkbox" bind:checked={settings.autoFetchBeforeSetup} /> 環境セットアップの直前に自動でベースクローンを <code>git fetch --prune</code> して最新化する</label>
          <label class="check"><input type="checkbox" bind:checked={settings.includeNodeModules} /> コピー時に <code>node_modules</code> も含める（npm ci を短縮）</label>
          <div class="cta">
            <button class="primary" onclick={saveSettings} disabled={busy}>保存</button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .kv { display: grid; grid-template-columns: 140px 1fr; gap: 10px; margin: 0 0 14px; font-size: 13px; align-items: center; }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; }
  .row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .mono { font-family: ui-monospace, Menlo, monospace; }
  .short { width: 100px; }
  .sub { font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); margin: 0 0 6px; }
  .status-list { display: flex; flex-direction: column; gap: 6px; }
  .status-row { display: grid; grid-template-columns: auto 1fr 130px auto; gap: 8px; align-items: center; }
  .order { display: inline-flex; gap: 2px; }
  .order .icon { font-size: 9px; padding: 2px 4px; }
  .add { text-align: left; }
  .note { font-size: 11px; margin: 10px 0 0; }
  code { font-family: ui-monospace, Menlo, monospace; }
  .settings-wrap {
    display: grid;
    grid-template-columns: 180px 1fr;
    flex: 1;
    min-height: 0;
    height: 100%;
  }
  .settings-nav {
    border-right: 1px solid var(--border);
    background: var(--panel);
    padding: 10px 8px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .settings-nav .grp {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--muted);
    padding: 8px 8px 4px;
  }
  .settings-nav button {
    text-align: left;
    background: transparent;
    border-color: transparent;
    padding: 6px 8px;
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .settings-nav button.on {
    background: var(--accent-bg);
    border-color: var(--accent);
    color: #fff;
  }
  .settings-panel {
    overflow-y: auto;
    min-height: 0;
    padding: 16px;
  }
  .panel-head {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .panel-head strong {
    color: #fff;
    font-size: 14px;
  }
  .ok {
    color: #22c55e;
    font-size: 12px;
  }
  .two-col {
    display: grid;
    grid-template-columns: 300px 1fr;
    gap: 16px;
    align-items: start;
  }
  .card {
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--panel);
    overflow: hidden;
  }
  .card.narrow {
    max-width: 760px;
  }
  .card > header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }
  .card > header strong {
    color: #fff;
    font-size: 13px;
  }
  .card .inner {
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .tpl {
    gap: 6px;
  }
  .tpl-row {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 10px;
    background: var(--panel-2);
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 4px 10px;
    text-align: left;
  }
  .tpl-row.sel {
    border-color: var(--accent);
  }
  .tpl-row .n {
    color: #fff;
    font-weight: 500;
  }
  .tpl-row .d {
    grid-column: 1 / -1;
    font-size: 11px;
    color: var(--muted);
  }
  .kv {
    display: grid;
    grid-template-columns: 130px 1fr;
    gap: 8px 12px;
    align-items: center;
    margin: 0;
  }
  .kv dt {
    font-size: 12px;
    color: var(--muted);
  }
  .kv dd {
    margin: 0;
    min-width: 0;
  }
  .mono {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 12px;
  }
  .hint {
    font-size: 11px;
    margin: 0;
  }
  .inline {
    display: flex;
    gap: 6px;
    align-items: center;
  }
  .inline-check,
  .check {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text);
  }
  .inline-check input,
  .check input {
    width: auto;
    margin: 0;
  }
  .cta {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    padding-top: 4px;
  }
  .fresh {
    color: #22c55e;
    font-size: 12px;
  }
  .bad {
    color: var(--danger);
    font-size: 12px;
  }
  td.stale {
    color: #f59e0b;
  }
  td.fresh {
    color: #22c55e;
  }
  .table-wrap {
    overflow-x: auto;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  th {
    text-align: left;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted);
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  td {
    padding: 7px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  td.num {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
  }
  td.mono.muted {
    max-width: 260px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  td.actions {
    white-space: nowrap;
    display: flex;
    gap: 4px;
    align-items: center;
  }
  .errs {
    margin: 0;
    padding: 8px 14px 8px 30px;
    color: var(--danger);
    font-size: 12px;
  }
  .add-bc {
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    border-top: 1px solid var(--border);
  }
  .add-bc strong {
    font-size: 12px;
    color: #fff;
  }
  .add-bc-row {
    display: grid;
    grid-template-columns: 150px 1fr 1fr auto;
    gap: 6px;
  }
  code {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 11px;
  }
</style>
