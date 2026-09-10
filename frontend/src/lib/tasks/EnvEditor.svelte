<script>
  // 作業環境（レイアウト / リポジトリ / ペイン割当）の編集フォーム。
  // テンプレート編集とタスク詳細の環境編集で共用する。bind した value を直接変更する
  // （Svelte 5 の深い反応性に任せる。内部に $state のコピーは持たない）。
  import { LAYOUTS, layoutOf } from '../api.js'

  let { value = $bindable(), baseClones = [], compact = false } = $props()

  const capacity = $derived(value ? layoutOf(value.layout || 'single').capacity : 0)

  // 配列が無いときに備えて初期化する（サーバは空配列を返すが、編集途中の新規作成に備える）
  function ensure() {
    if (!value) return false
    if (!Array.isArray(value.repos)) value.repos = []
    if (!Array.isArray(value.panes)) value.panes = []
    return true
  }

  function onLayoutChange(e) {
    if (!ensure()) return
    value.layout = e.currentTarget.value
    const cap = layoutOf(value.layout).capacity
    // 容量が減ったら範囲外のペインを落とす。増えても自動追加はしない
    value.panes = value.panes.filter((p) => p.slot < cap)
  }

  function addRepo() {
    if (!ensure()) return
    value.repos.push({ name: '', source: { kind: 'clone', url: '', baseCloneId: '' }, setupCommands: [] })
  }

  function removeRepo(i) {
    if (!ensure()) return
    const name = value.repos[i]?.name
    value.repos.splice(i, 1)
    // 消したリポジトリを指しているペインは作業フォルダ直下に戻す
    for (const p of value.panes) if (p.repoName === name) p.repoName = ''
  }

  function onSourceKind(r, e) {
    r.source.kind = e.currentTarget.value
    if (r.source.kind === 'baseCopy' && !r.source.baseCloneId && baseClones.length > 0) {
      r.source.baseCloneId = baseClones[0].id
    }
  }

  // 1 行 1 コマンドのテキストエリア ↔ 配列
  function linesOf(arr) {
    return (arr || []).join('\n')
  }
  function toLines(text) {
    return String(text || '')
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
  }

  function nextFreeSlot() {
    const used = new Set(value.panes.map((p) => p.slot))
    for (let s = 0; s < capacity; s++) if (!used.has(s)) return s
    return -1
  }

  function addPane() {
    if (!ensure()) return
    const slot = nextFreeSlot()
    if (slot < 0) return
    value.panes.push({ slot, repoName: '', commands: [] })
    value.panes.sort((a, b) => a.slot - b.slot)
  }

  function removePane(i) {
    if (!ensure()) return
    value.panes.splice(i, 1)
  }

  // ペインのコマンド: 行 → [{command, autoRun}]。autoRun はペイン単位のチェックで一括
  function paneAutoRun(p) {
    return (p.commands || []).length > 0 && p.commands.every((c) => c.autoRun)
  }
  function setPaneCommands(p, text) {
    const auto = p.commands?.length ? paneAutoRun(p) : true
    p.commands = toLines(text).map((command) => ({ command, autoRun: auto }))
  }
  function setPaneAutoRun(p, auto) {
    for (const c of p.commands || []) c.autoRun = auto
  }

  const sourceLabel = { clone: '⤓ git clone', baseCopy: '⧉ ベースクローンからコピー' }
</script>

{#if !value}
  <p class="muted">環境は未設定です。</p>
{:else}
  <div class="env" class:compact>
    <div class="row-h">
      <label class="field">
        <span>レイアウト</span>
        <select value={value.layout || 'single'} onchange={onLayoutChange}>
          {#each LAYOUTS as l}
            <option value={l.value}>{l.label}</option>
          {/each}
        </select>
      </label>
    </div>

    <section>
      <div class="sec-head">
        <strong>リポジトリ</strong>
        <span class="muted">作業フォルダ直下に <code>name/</code> として取得</span>
        <span class="spacer"></span>
        <button class="icon" type="button" onclick={addRepo}>＋ リポジトリを追加</button>
      </div>
      {#if !(value.repos || []).length}
        <p class="muted small">リポジトリなし（作業フォルダだけ作ります）</p>
      {/if}
      {#each value.repos || [] as r, i (i)}
        <div class="repo-row">
          <div class="repo-top">
            <input class="name" placeholder="名前（例: frontend）" bind:value={r.name} />
            <select value={r.source.kind} onchange={(e) => onSourceKind(r, e)}>
              {#each Object.entries(sourceLabel) as [k, label]}
                <option value={k}>{label}</option>
              {/each}
            </select>
            {#if r.source.kind === 'baseCopy'}
              <select bind:value={r.source.baseCloneId}>
                <option value="">（ベースクローンを選択）</option>
                {#each baseClones as bc (bc.id)}
                  <option value={bc.id}>{bc.name}</option>
                {/each}
              </select>
            {:else}
              <input class="url" placeholder="git@github.com:org/repo.git" bind:value={r.source.url} />
            {/if}
            <button class="icon danger" type="button" title="削除" onclick={() => removeRepo(i)}>✕</button>
          </div>
          <label class="field">
            <span>セットアップコマンド <span class="muted">（1 行 1 コマンド。取得後にリポジトリ内で順に実行）</span></span>
            <textarea
              rows={compact ? 1 : 2}
              placeholder="npm ci"
              value={linesOf(r.setupCommands)}
              onchange={(e) => (r.setupCommands = toLines(e.currentTarget.value))}
            ></textarea>
          </label>
        </div>
      {/each}
    </section>

    <section>
      <div class="sec-head">
        <strong>ペイン割当</strong>
        <span class="muted">{layoutOf(value.layout || 'single').label} · 最大 {capacity} ペイン</span>
        <span class="spacer"></span>
        <button class="icon" type="button" onclick={addPane} disabled={nextFreeSlot() < 0}>＋ ペインを追加</button>
      </div>
      {#if !(value.panes || []).length}
        <p class="muted small">ペインなし（作業フォルダ直下のペインを 1 つ作ります）</p>
      {/if}
      {#each value.panes || [] as p, i (p.slot)}
        <div class="pane-row">
          <span class="slot">pane {p.slot}</span>
          <select bind:value={p.repoName}>
            <option value="">作業フォルダ直下</option>
            {#each value.repos || [] as r}
              {#if r.name}
                <option value={r.name}>{r.name}</option>
              {/if}
            {/each}
          </select>
          <textarea
            rows="1"
            placeholder="起動コマンド（1 行 1 つ。例: npm run dev）"
            value={linesOf((p.commands || []).map((c) => c.command))}
            onchange={(e) => setPaneCommands(p, e.currentTarget.value)}
          ></textarea>
          <label class="inline-check" title="ペインを開いたときに自動で実行する">
            <input type="checkbox" checked={paneAutoRun(p)} onchange={(e) => setPaneAutoRun(p, e.currentTarget.checked)} />
            自動実行
          </label>
          <button class="icon danger" type="button" title="削除" onclick={() => removePane(i)}>✕</button>
        </div>
      {/each}
    </section>
  </div>
{/if}

<style>
  .env {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .env.compact {
    gap: 10px;
  }
  .row-h {
    display: flex;
    gap: 10px;
    align-items: flex-end;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
    color: var(--muted);
    min-width: 0;
  }
  .field select {
    width: auto;
    min-width: 160px;
  }
  .field textarea {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 12px;
  }
  section {
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--panel-2);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .sec-head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
  }
  .sec-head strong {
    color: #fff;
  }
  .sec-head .muted {
    font-size: 11px;
  }
  .small {
    font-size: 12px;
    margin: 0;
  }
  code {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 11px;
  }
  .repo-row {
    border-top: 1px solid var(--border);
    padding-top: 8px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .repo-top {
    display: grid;
    grid-template-columns: 140px auto 1fr auto;
    gap: 6px;
    align-items: center;
  }
  .repo-top select {
    width: auto;
  }
  .repo-top .url {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 12px;
  }
  .pane-row {
    border-top: 1px solid var(--border);
    padding-top: 8px;
    display: grid;
    grid-template-columns: 56px 150px 1fr auto auto;
    gap: 6px;
    align-items: center;
  }
  .pane-row .slot {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 11px;
    color: var(--muted);
  }
  .pane-row select {
    width: auto;
  }
  .pane-row textarea {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 12px;
    resize: vertical;
  }
  .inline-check {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    color: var(--muted);
    white-space: nowrap;
  }
  .inline-check input {
    width: auto;
    margin: 0;
  }
</style>
