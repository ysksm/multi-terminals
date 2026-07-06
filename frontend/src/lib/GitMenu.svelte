<script>
  import { onMount, tick } from 'svelte'
  import { api } from './api.js'
  import { menuKeyAction } from './gitMenu.js'

  let { workspaceId, paneId, onClose, onChanged } = $props()

  let branches = $state([])
  let loading = $state(true)
  let running = $state('') // '' | 'pull' | 'push' | 'fetch' | 'checkout'
  let errorMsg = $state('')
  let selectedIndex = $state(0)
  let root // メニュールート要素(フォーカス・外側クリック判定用)

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
      // メニューを開いたままキー操作(Esc/矢印/p,u,f)を続けられるよう、ルートへフォーカスを戻す。
      await tick()
      root?.focus()
    }
  }

  const runOp = (op) => run(op, () => api.paneGitOp(workspaceId, paneId, op))
  const checkout = (branch) => run('checkout', () => api.paneGitCheckout(workspaceId, paneId, branch))

  function onKeydown(e) {
    const action = menuKeyAction(e.key, { branchCount: branches.length, selectedIndex })
    if (!action) return
    e.preventDefault()
    e.stopPropagation()
    if (action.type === 'close') onClose?.()
    else if (action.type === 'move') selectedIndex = action.index
    else if (action.type === 'checkout') checkout(branches[selectedIndex].name)
    else if (action.type === 'op') runOp(action.op)
  }

  // メニュー外クリックで閉じる(開いたクリック自体はバブリング完了後なので拾わない)
  function onWindowClick(e) {
    if (root && !root.contains(e.target)) onClose?.()
  }

  onMount(() => {
    loadBranches()
    root?.focus()
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
    <ul>
      {#each branches as b, i (b.name)}
        <li>
          <button
            class="branch"
            class:selected={i === selectedIndex}
            disabled={!!running || b.isCurrent}
            onclick={() => checkout(b.name)}
            onmouseenter={() => (selectedIndex = i)}
          >
            <span class="check">{b.isCurrent ? '✓' : ''}</span>
            {b.name}
            {#if b.isRemote}<span class="remote">origin</span>{/if}
            {#if running === 'checkout' && i === selectedIndex}…{/if}
          </button>
        </li>
      {/each}
    </ul>
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
    min-width: 220px;
    max-width: 340px;
    padding: 6px;
    background: var(--panel-2);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
    outline: none;
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
    width: 1em;
  }
  .remote {
    margin-left: auto;
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
