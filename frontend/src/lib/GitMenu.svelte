<script>
  import { onMount, tick } from 'svelte'
  import { api } from './api.js'
  import { menuKeyAction, filterBranches } from './gitMenu.js'

  let { workspaceId, paneId, onClose, onChanged } = $props()

  let branches = $state([])
  let loading = $state(true)
  let running = $state('') // '' | 'pull' | 'push' | 'fetch' | 'checkout'
  let errorMsg = $state('')
  let selectedIndex = $state(0)
  let filter = $state('')
  let root // メニュールート要素(フォーカス・外側クリック判定用)
  let filterInput = $state() // 絞り込み入力欄(既定のフォーカス先)

  // 絞り込み後のブランチ一覧。selectedIndex はこの配列に対するインデックス。
  const filtered = $derived(filterBranches(branches, filter))

  // 絞り込みで件数が減ったとき selectedIndex が範囲外にならないようクランプする。
  $effect(() => {
    if (selectedIndex > filtered.length - 1) {
      selectedIndex = Math.max(filtered.length - 1, 0)
    }
  })

  async function loadBranches() {
    loading = true
    try {
      const res = await api.paneGitBranches(workspaceId, paneId)
      branches = res?.branches || []
      const cur = branches.findIndex((b) => b.isCurrent)
      selectedIndex = cur >= 0 ? cur : 0
    } catch (e) {
      errorMsg = e.message || String(e)
    } finally {
      loading = false
    }
  }

  async function run(kind, fn) {
    if (running) return
    running = kind
    errorMsg = ''
    try {
      await fn()
      await loadBranches()
      onChanged?.()
    } catch (e) {
      errorMsg = e.message || String(e)
    } finally {
      running = ''
      // ブラウザは disabled になった要素(直前にクリックしたボタン)から自動的にフォーカスを外す。
      // メニューを開いたままキー操作を続けられるよう、絞り込み入力へフォーカスを戻す。
      await tick()
      ;(filterInput ?? root)?.focus()
    }
  }

  const runOp = (op) => run(op, () => api.paneGitOp(workspaceId, paneId, op))
  const checkout = (branch) => run('checkout', () => api.paneGitCheckout(workspaceId, paneId, branch))

  // action を実行する。呼び出し元がイベントの preventDefault / stopPropagation を担う。
  function applyAction(action) {
    if (action.type === 'close') onClose?.()
    else if (action.type === 'move') selectedIndex = action.index
    else if (action.type === 'checkout') checkout(filtered[selectedIndex].name)
    else if (action.type === 'op') runOp(action.op)
  }

  // ルート(入力欄以外)にフォーカスがあるときのキー操作。p/u/f も ops として効く。
  function onKeydown(e) {
    const action = menuKeyAction(e.key, { branchCount: filtered.length, selectedIndex })
    if (!action) return
    e.preventDefault()
    e.stopPropagation()
    applyAction(action)
  }

  // 絞り込み入力欄でのキー操作。移動・checkout・close だけを横取りし、
  // 文字キー(p/u/f を含む)は入力へ通して絞り込みに使う。ops キーは発火させない。
  function onFilterKeydown(e) {
    const action = menuKeyAction(e.key, { branchCount: filtered.length, selectedIndex })
    // 入力欄のキーはルートの onKeydown に伝播させない(二重処理・ops 誤発火の防止)。
    e.stopPropagation()
    if (!action || action.type === 'op') return
    e.preventDefault()
    applyAction(action)
  }

  // メニュー外クリックで閉じる(開いたクリック自体はバブリング完了後なので拾わない)
  function onWindowClick(e) {
    if (root && !root.contains(e.target)) onClose?.()
  }

  onMount(() => {
    loadBranches()
    ;(filterInput ?? root)?.focus()
  })
</script>

<svelte:window onclick={onWindowClick} />

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="git-menu" bind:this={root} tabindex="-1" onkeydown={onKeydown} role="menu">
  <div class="ops">
    <button onclick={() => runOp('pull')} disabled={!!running}>
      {running === 'pull' ? '…' : '⬇'} Pull
    </button>
    <button onclick={() => runOp('push')} disabled={!!running}>
      {running === 'push' ? '…' : '⬆'} Push
    </button>
    <button onclick={() => runOp('fetch')} disabled={!!running}>
      {running === 'fetch' ? '…' : '⟳'} Fetch
    </button>
  </div>
  <hr />
  {#if loading}
    <div class="muted">読み込み中…</div>
  {:else if branches.length === 0}
    <div class="muted">ブランチなし</div>
  {:else}
    <input
      class="filter"
      type="text"
      placeholder="ブランチを絞り込み…"
      bind:value={filter}
      bind:this={filterInput}
      oninput={() => (selectedIndex = 0)}
      onkeydown={onFilterKeydown}
    />
    {#if filtered.length === 0}
      <div class="muted">一致するブランチなし</div>
    {:else}
      <ul>
        {#each filtered as b, i (b.name)}
          <li>
            <button
              class="branch"
              class:selected={i === selectedIndex}
              disabled={!!running || b.isCurrent}
              onclick={() => checkout(b.name)}
              onmouseenter={() => (selectedIndex = i)}
            >
              <span class="check">{b.isCurrent ? '✓' : ''}</span>
              <span class="name">{b.name}</span>
              {#if b.isRemote}<span class="remote">origin</span>{/if}
              {#if running === 'checkout' && i === selectedIndex}…{/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
  {#if errorMsg}
    <div class="error">{errorMsg}</div>
  {/if}
</div>

<style>
  .git-menu {
    position: absolute;
    top: 100%;
    left: 0;
    z-index: 30;
    min-width: 300px;
    max-width: 520px;
    padding: 6px;
    background: var(--panel-2);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
    outline: none;
  }
  .filter {
    width: 100%;
    box-sizing: border-box;
    margin-bottom: 6px;
    padding: 4px 6px;
    font-size: 12px;
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 4px;
    outline: none;
  }
  .filter:focus {
    border-color: var(--accent-bg);
  }
  .ops {
    display: flex;
    gap: 4px;
  }
  .ops button {
    flex: 1;
    font-size: 12px;
    padding: 4px 6px;
  }
  hr {
    border: none;
    border-top: 1px solid var(--border);
    margin: 6px 0;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 220px;
    overflow-y: auto;
  }
  .branch {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    padding: 4px 6px;
    border-radius: 4px;
    font-size: 12px;
    cursor: pointer;
    color: var(--text);
  }
  .branch.selected {
    background: var(--accent-bg);
  }
  .branch:disabled {
    cursor: default;
    opacity: 0.7;
  }
  .check {
    flex: none;
    width: 1em;
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .remote {
    flex: none;
    font-size: 10px;
    opacity: 0.6;
  }
  .muted {
    font-size: 12px;
    opacity: 0.6;
    padding: 4px 6px;
    color: var(--muted);
  }
  .error {
    margin-top: 6px;
    padding: 4px 6px;
    font-size: 11px;
    color: #fca5a5;
    white-space: pre-wrap;
    word-break: break-all;
    border-top: 1px solid var(--border);
  }
</style>
