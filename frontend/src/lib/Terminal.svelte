<script>
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'

  // paneId が設定されると WebSocket を接続してライブ端末を表示する。
  let { paneId } = $props()

  let host // 端末を描画する DOM
  let term
  let fit
  let ws
  let status = $state('connecting')

  function wsURL(id) {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/api/panes/${id}/io`
  }

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

    ws = new WebSocket(wsURL(id))
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      status = 'connected'
      sendResize()
    }
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(ev.data))
      } else if (typeof ev.data === 'string') {
        term.write(ev.data)
      }
    }
    ws.onclose = () => {
      status = 'closed'
      term?.write('\r\n\x1b[33m[session closed]\x1b[0m\r\n')
    }
    ws.onerror = () => {
      status = 'error'
    }

    term.onData((data) => {
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }))
      }
    })

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
    if (ws?.readyState === WebSocket.OPEN && term) {
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    }
  }

  onMount(() => {
    let ro
    if (paneId) ro = connect(paneId)
    return () => {
      ro?.disconnect()
      ws?.close()
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
