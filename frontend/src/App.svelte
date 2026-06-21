<script>
  import { onMount } from 'svelte'
  import { api, LAYOUTS, layoutOf } from './lib/api.js'
  import Terminal from './lib/Terminal.svelte'

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
      openedPaneIds = new Set() // 別ワークスペース選択で端末接続をリセット
    })
  }

  async function reloadCurrent() {
    if (current) current = await api.getWorkspace(current.id)
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
  }

  function submitAddPane() {
    if (!paneDir.trim()) {
      error = '作業ディレクトリを入力してください'
      return
    }
    const commands = paneCmds
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((command) => ({ command, autoRun: paneAutoRun }))
    guard(async () => {
      await api.addPane(current.id, paneDir.trim(), addingSlot, commands)
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

  onMount(() => {
    guard(async () => {
      await refreshList()
      const last = await api.lastOpened()
      if (last?.found && last.workspace) {
        current = last.workspace
      }
    })
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

<div class="app">
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
            <button class:active={current?.id === w.id} onclick={() => select(w.id)}>
              <span class="name">{w.name}</span>
              <span class="badge">{layoutOf(w.layout).label}</span>
            </button>
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
      <div class="empty">左でワークスペースを選択 / 作成してください</div>
    {:else}
      <div class="toolbar">
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
          <div class="cell">
            {#if cell.pane}
              <div class="cell-head">
                <span class="dir" title={cell.pane.directory}>{cell.pane.directory}</span>
                <span class="cell-actions">
                  <button class="icon" title="最大化/戻す" onclick={() => toggleMaximize(cell.pane.id)}>⤢</button>
                  <button class="icon" title="削除" onclick={() => removePane(cell.pane.id)}>✕</button>
                </span>
              </div>
              <div class="cell-body">
                {#if openedPaneIds.has(cell.pane.id)}
                  {#key cell.pane.id}
                    <Terminal paneId={cell.pane.id} />
                  {/key}
                {:else}
                  <div class="not-open">
                    <p>未起動</p>
                    {#if cell.pane.commands?.length}
                      <ul class="cmds">
                        {#each cell.pane.commands as c}
                          <li>{c.autoRun ? '▶' : '·'} {c.command}</li>
                        {/each}
                      </ul>
                    {/if}
                    <small class="muted">「開く」で起動します</small>
                  </div>
                {/if}
              </div>
            {:else if addingSlot === cell.slot}
              <div class="add-form">
                <h3>スロット {cell.slot} にペイン追加</h3>
                <label>作業ディレクトリ
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
                  <button class="primary" onclick={submitAddPane} disabled={busy}>追加</button>
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
