// 端末転送の抽象。Wails デスクトップ環境ではネイティブバインディング
// (window.go + runtime events)、ブラウザでは従来の WebSocket を使う。

export function isDesktop() {
  return (
    typeof window !== 'undefined' &&
    !!window.runtime &&
    !!window.go &&
    !!window.go.main &&
    !!window.go.main.App
  )
}

export function paneWsURL(location, paneId) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/api/panes/${paneId}/io`
}

export function b64ToBytes(b64) {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

// connectPane は端末1ペイン分の双方向接続を確立し、
// { send, resize, close } を返す。
//   onData(Uint8Array|string): 出力受信
//   onClose(): セッション終了/切断
export function connectPane(paneId, { onData, onClose }) {
  if (isDesktop()) {
    return connectDesktop(paneId, { onData, onClose })
  }
  return connectBrowser(paneId, { onData, onClose })
}

function connectDesktop(paneId, { onData, onClose }) {
  const App = window.go.main.App
  const dataEvent = `pane:${paneId}`
  const doneEvent = `pane:${paneId}:done`

  window.runtime.EventsOn(dataEvent, (b64) => onData(b64ToBytes(b64)))
  window.runtime.EventsOn(doneEvent, () => onClose && onClose())

  // 購読開始。失敗時は onClose で通知。
  Promise.resolve(App.PaneSubscribe(paneId)).catch(() => onClose && onClose())

  return {
    send(data) {
      App.PaneWrite(paneId, data)
    },
    resize(cols, rows) {
      App.PaneResize(paneId, cols, rows)
    },
    close() {
      window.runtime.EventsOff(dataEvent)
      window.runtime.EventsOff(doneEvent)
      App.PaneUnsubscribe(paneId)
    },
  }
}

function connectBrowser(paneId, { onData, onClose }) {
  const ws = new WebSocket(paneWsURL(window.location, paneId))
  ws.binaryType = 'arraybuffer'

  ws.onmessage = (ev) => {
    if (ev.data instanceof ArrayBuffer) onData(new Uint8Array(ev.data))
    else if (typeof ev.data === 'string') onData(ev.data)
  }
  ws.onclose = () => onClose && onClose()

  return {
    send(data) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }))
      }
    },
    resize(cols, rows) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    },
    close() {
      ws.close()
    },
    _ws: ws, // Terminal.svelte が onopen で初回リサイズするために参照
  }
}
