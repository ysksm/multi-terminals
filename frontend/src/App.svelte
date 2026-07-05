<script>
  import { onMount } from 'svelte'
  import { api, LAYOUTS, layoutOf } from './lib/api.js'
  import Terminal from './lib/Terminal.svelte'
  import { neighborSlot } from './lib/paneNav.js'

  let workspaces = $state([])
  let current = $state(null) // 選択中の WorkspaceDTO
  let openedPaneIds = $state(new Set()) // ライブセッションを持つ pane
  let error = $state('')
  let busy = $state(false)

  // 新規ワークスペースフォーム
  let newName = $state('')
  let newLayout = $state('split_vertical')

  // ペイン追加フォーム（slot をキーに開閉）
  let addingSlot = $state(null)
  let paneDir = $state('')
  let paneCmds = $state('')
  let paneAutoRun = $state(true)
  let paneTitle = $state('')
  let paneRepoUrl = $state('')
  let lastAutoDir = ''

  // ペイン毎の git 情報（paneId -> {isRepo, branch, dirty}）
  let paneGit = $state({})

  // ペインタイトルインライン編集
  let editingTitlePaneId = $state(null)
  let titleDraft = $state('')

  // ペイン内容（作業ディレクトリ・起動コマンド）編集
  let editingPaneId = $state(null)
  let editDir = $state('')
  let editCmds = $state('')
  let editAutoRun = $state(true)

  // サイドバー折りたたみ
  let sidebarCollapsed = $state(localStorage.getItem('mt.sidebarCollapsed') === '1')

  // 削除確認
  let confirmingDeleteId = $state(null)

  // アクティブペイン
  let activePaneId = $state(null)

  const layout = $derived(current ? layoutOf(current.layout) : layoutOf('single'))
  const maximized = $derived(current?.maximizedPaneId || null)

  function paneAtSlot(slot) {
    return current?.panes?.find((p) => p.slot === slot) || null
  }

  async function guard(fn) {
    error = ''
    busy = true
    try {
      await fn()
    } catch (e) {
      error = e.message || String(e)
    } finally {
      busy = false
    }
  }

  async function refreshList() {
    workspaces = (await api.listWorkspaces()) || []
  }

  async function select(id) {
    await guard(async () => {
      current = await api.getWorkspace(id)
      activePaneId = current?.lastActivePaneId ?? current?.panes?.[0]?.id ?? null
      // サーバー側で生存しているセッションへ自動再接続（resume）
      await syncLiveSessions()
      await refreshGitInfo()
    })
  }

  async function reloadCurrent() {
    if (current) {
      current = await api.getWorkspace(current.id)
      activePaneId = current?.lastActivePaneId ?? current?.panes?.[0]?.id ?? null
      await refreshGitInfo()
    }
  }

  // 各ペインの git 情報（ブランチ・変更有無）を取得する。失敗したペインは
  // バッジ非表示にするだけで、全体のエラーにはしない。
  async function refreshGitInfo() {
    if (!current) {
      paneGit = {}
      return
    }
    const entries = await Promise.all(
      current.panes.map(async (p) => {
        try {
          return [p.id, await api.paneGit(current.id, p.id)]
        } catch {
          return [p.id, null]
        }
      })
    )
    paneGit = Object.fromEntries(entries)
  }

  // サーバー上で生きているセッションを取得し、現ワークスペースの該当ペインを
  // openedPaneIds にセットする。端末コンポーネントが自動で再接続し、
  // スクロールバック（直近の画面）が復元される。
  async function syncLiveSessions() {
    if (!current) {
      openedPaneIds = new Set()
      return
    }
    const res = await api.listSessions()
    const live = new Set(res?.paneIds || [])
    openedPaneIds = new Set(current.panes.filter((p) => live.has(p.id)).map((p) => p.id))
  }

  function createWorkspace() {
    if (!newName.trim()) {
      error = 'ワークスペース名を入力してください'
      return
    }
    guard(async () => {
      const res = await api.createWorkspace(newName.trim(), newLayout)
      newName = ''
      await refreshList()
      await select(res.id)
    })
  }

  function changeLayout(value) {
    guard(async () => {
      await api.patchWorkspace(current.id, { layout: value })
      await reloadCurrent()
    })
  }

  function startAddPane(slot) {
    addingSlot = slot
    paneDir = ''
    paneCmds = ''
    paneAutoRun = true
    paneTitle = ''
    paneRepoUrl = ''
    lastAutoDir = ''
  }

  // リポジトリ URL からリポジトリ名を推定する（末尾の .git / 区切りを除去）。
  function repoNameFromUrl(url) {
    const m = url
      .trim()
      .replace(/\/+$/, '')
      .match(/([^/:]+?)(\.git)?$/)
    return m ? m[1] : ''
  }

  // URL 入力に応じて clone 先を自動補完する。ユーザーが手で書き換えた
  // ディレクトリは上書きしない（直前の自動補完値のときだけ更新）。
  function onRepoUrlInput() {
    const name = repoNameFromUrl(paneRepoUrl)
    if (!name) return
    if (!paneDir.trim() || paneDir === lastAutoDir) {
      paneDir = `~/src/github/${name}`
      lastAutoDir = paneDir
    }
  }

  function submitAddPane() {
    if (!paneDir.trim()) {
      error = '作業ディレクトリを入力してください'
      return
    }
    const repoUrl = paneRepoUrl.trim()
    const commands = paneCmds
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((command) => ({ command, autoRun: paneAutoRun }))
    guard(async () => {
      let dir = paneDir.trim()
      if (repoUrl) {
        const res = await api.cloneRepo(repoUrl, dir)
        dir = res.path
      }
      await api.addPane(current.id, dir, addingSlot, commands, paneTitle.trim())
      addingSlot = null
      await reloadCurrent()
    })
  }

  function removePane(paneId) {
    guard(async () => {
      await api.removePane(current.id, paneId)
      openedPaneIds.delete(paneId)
      openedPaneIds = new Set(openedPaneIds)
      await reloadCurrent()
    })
  }

  function openWorkspace() {
    guard(async () => {
      const res = await api.open(current.id)
      const ids = (res?.panes || []).map((p) => p.paneId)
      openedPaneIds = new Set(ids)
      await reloadCurrent()
    })
  }

  function toggleMaximize(paneId) {
    guard(async () => {
      if (maximized === paneId) {
        await api.restoreLayout(current.id)
      } else {
        await api.maximizePane(current.id, paneId)
      }
      await reloadCurrent()
    })
  }

  function toggleSidebar() {
    sidebarCollapsed = !sidebarCollapsed
    localStorage.setItem('mt.sidebarCollapsed', sidebarCollapsed ? '1' : '0')
  }

  function onKey(e) {
    if (!(e.ctrlKey && e.shiftKey) || e.altKey || e.metaKey) return
    const dirs = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }
    const dir = dirs[e.key]
    if (!dir) return
    if (!current || maximized) return
    e.preventDefault()
    e.stopPropagation()
    const lay = layoutOf(current.layout)
    const activePane = current.panes.find((p) => p.id === activePaneId) || current.panes[0]
    if (!activePane) return
    const target = neighborSlot(activePane.slot, lay.cols, lay.rows, dir)
    if (target == null) return
    const next = current.panes.find((p) => p.slot === target)
    if (next) activePaneId = next.id
  }

  function startEditTitle(pane) {
    editingTitlePaneId = pane.id
    titleDraft = pane.title || ''
  }
  function cancelEditTitle() {
    editingTitlePaneId = null
  }
  function commitEditTitle(paneId) {
    // 編集中の pane でなければ何もしない。input 除去時に発火する blur の再入
    // （Enter→blur の二重 PUT、Escape キャンセル後の意図しない保存）を防ぐ。
    if (editingTitlePaneId !== paneId) return
    const next = titleDraft.trim()
    editingTitlePaneId = null
    guard(async () => {
      await api.setPaneTitle(current.id, paneId, next)
      await reloadCurrent()
    })
  }

  // ペイン内容の編集（作業ディレクトリ・起動コマンド）。タイトルはセルヘッダの
  // クリックで従来どおりインライン編集できる。
  function startEditPane(pane) {
    editingPaneId = pane.id
    editDir = pane.directory || ''
    editCmds = (pane.commands || []).map((c) => c.command).join('\n')
    editAutoRun = pane.commands?.length ? pane.commands.every((c) => c.autoRun) : true
  }
  function cancelEditPane() {
    editingPaneId = null
  }
  function submitEditPane(paneId) {
    if (!editDir.trim()) {
      error = '作業ディレクトリを入力してください'
      return
    }
    const commands = editCmds
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((command) => ({ command, autoRun: editAutoRun }))
    guard(async () => {
      await api.setPaneDirectory(current.id, paneId, editDir.trim())
      await api.setPaneCommands(current.id, paneId, commands)
      editingPaneId = null
      await reloadCurrent()
    })
  }

  // ペインの作業ディレクトリを Finder / VS Code で開く（バックエンド経由）。
  function openPaneIn(paneId, target) {
    guard(async () => {
      await api.openPaneIn(current.id, paneId, target)
    })
  }

  // 動的に挿入される input は autofocus が効かないため action で明示フォーカスする。
  function focusOnMount(el) {
    el.focus()
    el.select?.()
  }

  onMount(() => {
    guard(async () => {
      await refreshList()
      const last = await api.lastOpened()
      if (last?.found && last.workspace) {
        current = last.workspace
        activePaneId = current?.lastActivePaneId ?? current?.panes?.[0]?.id ?? null
        // リロード/再訪時に生存セッションへ自動再接続する
        await syncLiveSessions()
        await refreshGitInfo()
      }
    })
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  })

  // 表示するスロット一覧（最大化中はそのペインのみ）
  const slots = $derived.by(() => {
    if (!current) return []
    if (maximized) {
      const p = current.panes.find((x) => x.id === maximized)
      return p ? [{ slot: p.slot, pane: p }] : []
    }
    const out = []
    for (let i = 0; i < layout.capacity; i++) {
      out.push({ slot: i, pane: paneAtSlot(i) })
    }
    return out
  })
</script>

<div class="app" class:sidebar-collapsed={sidebarCollapsed}>
  <aside class="sidebar">
    <h1>multi-terminals</h1>

    <section class="create">
      <h2>新規ワークスペース</h2>
      <input placeholder="名前" bind:value={newName} />
      <select bind:value={newLayout}>
        {#each LAYOUTS as l}
          <option value={l.value}>{l.label}</option>
        {/each}
      </select>
      <button onclick={createWorkspace} disabled={busy}>作成</button>
    </section>

    <section class="list">
      <h2>ワークスペース</h2>
      {#if workspaces.length === 0}
        <p class="muted">まだありません</p>
      {/if}
      <ul>
        {#each workspaces as w}
          <li>
            <button class="ws-select" class:active={current?.id === w.id} onclick={() => select(w.id)}>
              <span class="name">{w.name}</span>
              <span class="badge">{layoutOf(w.layout).label}</span>
            </button>
            {#if confirmingDeleteId === w.id}
              <button
                class="icon danger"
                onclick={() =>
                  guard(async () => {
                    await api.deleteWorkspace(w.id)
                    if (current?.id === w.id) current = null
                    confirmingDeleteId = null
                    await refreshList()
                  })}
              >削除？</button>
              <button class="icon" onclick={() => (confirmingDeleteId = null)}>取消</button>
            {:else}
              <button
                class="icon"
                title="削除"
                onclick={(e) => {
                  e.stopPropagation()
                  confirmingDeleteId = w.id
                }}
              >✕</button>
            {/if}
          </li>
        {/each}
      </ul>
    </section>
  </aside>

  <main class="workspace">
    {#if error}
      <div class="error" role="alert">{error}</div>
    {/if}

    {#if !current}
      <div class="empty-bar">
        <button
          class="icon sidebar-toggle"
          onclick={toggleSidebar}
          aria-label="サイドバー切替"
          aria-expanded={!sidebarCollapsed}
        >☰</button>
      </div>
      <div class="empty">左でワークスペースを選択 / 作成してください</div>
    {:else}
      <div class="toolbar">
        <button
          class="icon sidebar-toggle"
          onclick={toggleSidebar}
          aria-label="サイドバー切替"
          aria-expanded={!sidebarCollapsed}
        >☰</button>
        <strong>{current.name}</strong>
        <select
          value={current.layout}
          onchange={(e) => changeLayout(e.currentTarget.value)}
          disabled={busy}
        >
          {#each LAYOUTS as l}
            <option value={l.value}>{l.label}</option>
          {/each}
        </select>
        <button class="primary" onclick={openWorkspace} disabled={busy}>▶ 開く（PTY 起動）</button>
        {#if maximized}
          <button onclick={() => toggleMaximize(maximized)} disabled={busy}>⤢ 元に戻す</button>
        {/if}
        <span class="spacer"></span>
        <span class="muted">{current.panes.length} / {layout.capacity} ペイン</span>
      </div>

      <div
        class="grid"
        style="grid-template-columns: repeat({maximized ? 1 : layout.cols}, 1fr); grid-template-rows: repeat({maximized ? 1 : layout.rows}, 1fr);"
      >
        {#each slots as cell (cell.slot)}
          <div class="cell" class:active-cell={cell.pane && cell.pane.id === activePaneId}>
            {#if cell.pane}
              <div class="cell-head">
                {#if editingTitlePaneId === cell.pane.id}
                  <input
                    class="title-edit"
                    bind:value={titleDraft}
                    onkeydown={(e) => {
                      if (e.key === 'Enter') commitEditTitle(cell.pane.id)
                      else if (e.key === 'Escape') cancelEditTitle()
                    }}
                    onblur={() => commitEditTitle(cell.pane.id)}
                    use:focusOnMount
                  />
                {:else}
                  <span
                    class="dir"
                    title={cell.pane.directory}
                    role="button"
                    tabindex="0"
                    onclick={() => startEditTitle(cell.pane)}
                    onkeydown={(e) => { if (e.key === 'Enter') startEditTitle(cell.pane) }}
                  >{cell.pane.title || cell.pane.directory}</span>
                {/if}
                {#if paneGit[cell.pane.id]?.isRepo}
                  <span
                    class="git-badge"
                    class:dirty={paneGit[cell.pane.id].dirty}
                    title={paneGit[cell.pane.id].dirty ? '未コミットの変更あり' : 'クリーン'}
                  >⎇ {paneGit[cell.pane.id].branch}{paneGit[cell.pane.id].dirty ? '*' : ''}</span>
                {/if}
                <span class="cell-actions">
                  <button class="icon" title="Finderで開く" onclick={() => openPaneIn(cell.pane.id, 'finder')}>📁</button>
                  <button class="icon" title="VSCodeで開く" onclick={() => openPaneIn(cell.pane.id, 'vscode')}>{'</>'}</button>
                  {#if paneGit[cell.pane.id]?.isRepo}
                    <button class="icon" title="リモート(GitHub)を開く" onclick={() => openPaneIn(cell.pane.id, 'github')}>🌐</button>
                  {/if}
                  <button class="icon" title="編集" onclick={() => startEditPane(cell.pane)}>✎</button>
                  <button class="icon" title="最大化/戻す" onclick={() => toggleMaximize(cell.pane.id)}>⤢</button>
                  <button class="icon" title="削除" onclick={() => removePane(cell.pane.id)}>✕</button>
                </span>
              </div>
              <div class="cell-body">
                {#if editingPaneId === cell.pane.id}
                  <div class="add-form">
                    <h3>ペインを編集</h3>
                    <label>作業ディレクトリ
                      <input placeholder="/path/to/project" bind:value={editDir} use:focusOnMount />
                    </label>
                    <label>起動コマンド（1行1コマンド）
                      <textarea rows="3" placeholder="npm run dev" bind:value={editCmds}></textarea>
                    </label>
                    <label class="row">
                      <input type="checkbox" bind:checked={editAutoRun} />
                      開いたとき自動実行する
                    </label>
                    <div class="row">
                      <button class="primary" onclick={() => submitEditPane(cell.pane.id)} disabled={busy}>保存</button>
                      <button onclick={cancelEditPane}>キャンセル</button>
                    </div>
                    {#if openedPaneIds.has(cell.pane.id)}
                      <small class="muted">変更は次回「開く」で反映されます</small>
                    {/if}
                  </div>
                {:else if openedPaneIds.has(cell.pane.id)}
                  {#key cell.pane.id}
                    <Terminal
                      paneId={cell.pane.id}
                      active={cell.pane.id === activePaneId}
                      onActivate={() => (activePaneId = cell.pane.id)}
                    />
                  {/key}
                {:else}
                  <!-- クリックで「開く」と同じ動作（ワークスペースの未起動ペインを起動） -->
                  <div
                    class="not-open"
                    role="button"
                    tabindex="0"
                    onclick={openWorkspace}
                    onkeydown={(e) => { if (e.key === 'Enter') openWorkspace() }}
                  >
                    <p>未起動</p>
                    {#if cell.pane.commands?.length}
                      <ul class="cmds">
                        {#each cell.pane.commands as c}
                          <li>{c.autoRun ? '▶' : '·'} {c.command}</li>
                        {/each}
                      </ul>
                    {/if}
                    <small class="muted">クリックで起動します</small>
                  </div>
                {/if}
              </div>
            {:else if addingSlot === cell.slot}
              <div class="add-form">
                <h3>スロット {cell.slot} にペイン追加</h3>
                <label>タイトル（任意）
                  <input placeholder="例: API サーバー" bind:value={paneTitle} />
                </label>
                <label>リポジトリ URL から clone（任意）
                  <input
                    placeholder="https://github.com/user/repo.git"
                    bind:value={paneRepoUrl}
                    oninput={onRepoUrlInput}
                  />
                </label>
                <label>{paneRepoUrl.trim() ? 'clone 先ディレクトリ' : '作業ディレクトリ'}
                  <input placeholder="/path/to/project" bind:value={paneDir} />
                </label>
                <label>起動コマンド（1行1コマンド）
                  <textarea rows="3" placeholder="npm run dev" bind:value={paneCmds}></textarea>
                </label>
                <label class="row">
                  <input type="checkbox" bind:checked={paneAutoRun} />
                  開いたとき自動実行する
                </label>
                <div class="row">
                  <button class="primary" onclick={submitAddPane} disabled={busy}>
                    {paneRepoUrl.trim() ? 'Clone して追加' : '追加'}
                  </button>
                  <button onclick={() => (addingSlot = null)}>キャンセル</button>
                </div>
              </div>
            {:else}
              <button class="empty-slot" onclick={() => startAddPane(cell.slot)}>＋ ペインを追加</button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </main>
</div>
