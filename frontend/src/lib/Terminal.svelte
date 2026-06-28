<script>
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'
  import { connectPane } from './termTransport.js'

  // paneId が設定されると接続してライブ端末を表示する。
  let { paneId } = $props()

  let host
  let term
  let fit
  let conn
  let status = $state('connecting')

  function connect(id) {
    term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: { background: '#1e1e1e' },
    })
    fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    fit.fit()

    conn = connectPane(id, {
      onData: (data) => term.write(data),
      onClose: () => {
        status = 'closed'
        term?.write('\r\n\x1b[33m[session closed]\x1b[0m\r\n')
      },
    })

    // ブラウザ(WS)では onopen を待って初回リサイズ。デスクトップは即送れる。
    if (conn._ws) {
      conn._ws.addEventListener('open', () => {
        status = 'connected'
        sendResize()
      })
    } else {
      status = 'connected'
      sendResize()
    }

    term.onData((data) => conn.send(data))

    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
        sendResize()
      } catch {
        // 端末未初期化時などは無視
      }
    })
    ro.observe(host)
    return ro
  }

  function sendResize() {
    if (conn && term) conn.resize(term.cols, term.rows)
  }

  onMount(() => {
    let ro
    if (paneId) ro = connect(paneId)
    return () => {
      ro?.disconnect()
      conn?.close()
      term?.dispose()
    }
  })
</script>

<div class="term-wrap">
  <div class="term-host" bind:this={host}></div>
  {#if status !== 'connected'}
    <div class="term-status">{status}</div>
  {/if}
</div>

<style>
  .term-wrap {
    position: relative;
    width: 100%;
    height: 100%;
    background: #1e1e1e;
    overflow: hidden;
  }
  .term-host {
    width: 100%;
    height: 100%;
    padding: 4px;
    box-sizing: border-box;
  }
  .term-status {
    position: absolute;
    top: 4px;
    right: 8px;
    font-size: 11px;
    color: #888;
    background: rgba(0, 0, 0, 0.5);
    padding: 1px 6px;
    border-radius: 3px;
    pointer-events: none;
  }
</style>
