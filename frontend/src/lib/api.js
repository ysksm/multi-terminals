// Go バックエンドの REST API クライアント。Vite プロキシ経由で同一オリジン（/api）。

async function req(method, path, body) {
  const opts = { method, headers: {} }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(path, opts)
  if (!res.ok) {
    let detail = ''
    try {
      const j = await res.json()
      detail = j.error || ''
    } catch {
      // ignore
    }
    throw new Error(`${method} ${path} -> ${res.status}${detail ? ': ' + detail : ''}`)
  }
  if (res.status === 204) return null
  const text = await res.text()
  return text ? JSON.parse(text) : null
}

export const api = {
  listWorkspaces: () => req('GET', '/api/workspaces'),
  createWorkspace: (name, layout) => req('POST', '/api/workspaces', { name, layout }),
  getWorkspace: (id) => req('GET', `/api/workspaces/${id}`),
  patchWorkspace: (id, patch) => req('PATCH', `/api/workspaces/${id}`, patch),
  maximizePane: (id, paneId) => req('POST', `/api/workspaces/${id}/maximize`, { paneId }),
  restoreLayout: (id) => req('POST', `/api/workspaces/${id}/restore`),
  setActivePane: (id, paneId) => req('POST', `/api/workspaces/${id}/active-pane`, { paneId }),
  lastOpened: () => req('GET', '/api/last-opened'),
  listSessions: () => req('GET', '/api/sessions'),
  addPane: (id, directory, slot, commands, title, remoteHost = '') =>
    req('POST', `/api/workspaces/${id}/panes`, { directory, slot, commands, title, remoteHost }),
  removePane: (id, paneId) => req('DELETE', `/api/workspaces/${id}/panes/${paneId}`),
  setPaneDirectory: (id, paneId, directory) =>
    req('PUT', `/api/workspaces/${id}/panes/${paneId}/directory`, { directory }),
  setPaneTitle: (id, paneId, title) =>
    req('PUT', `/api/workspaces/${id}/panes/${paneId}/title`, { title }),
  setPaneRemoteHost: (id, paneId, remoteHost) =>
    req('PUT', `/api/workspaces/${id}/panes/${paneId}/remote-host`, { remoteHost }),
  setPaneCommands: (id, paneId, commands) =>
    req('PUT', `/api/workspaces/${id}/panes/${paneId}/commands`, { commands }),
  openPaneIn: (id, paneId, target) =>
    req('POST', `/api/workspaces/${id}/panes/${paneId}/open-in`, { target }),
  paneGit: (id, paneId) => req('GET', `/api/workspaces/${id}/panes/${paneId}/git`),
  cloneRepo: (url, dest) => req('POST', '/api/repos/clone', { url, dest }),
  open: (id) => req('POST', `/api/workspaces/${id}/open`),
  deleteWorkspace: (id) => req('DELETE', `/api/workspaces/${id}`),
  // リモート実行の鍵管理
  remoteIdentity: () => req('GET', '/api/remote/identity'),
  createIdentity: () => req('POST', '/api/remote/identity'),
  regenerateIdentity: () => req('POST', '/api/remote/identity/regenerate'),
  deleteIdentity: () => req('DELETE', '/api/remote/identity'),
  listAuthorizedKeys: () => req('GET', '/api/remote/authorized-keys'),
  addAuthorizedKey: (key, comment) => req('POST', '/api/remote/authorized-keys', { key, comment }),
  removeAuthorizedKey: (key) => req('DELETE', `/api/remote/authorized-keys?key=${encodeURIComponent(key)}`),
}

// レイアウトプリセットの定義（バックエンドの値と一致させる）。
export const LAYOUTS = [
  { value: 'single', label: '1画面', capacity: 1, cols: 1, rows: 1 },
  { value: 'split_vertical', label: '左右2分割', capacity: 2, cols: 2, rows: 1 },
  { value: 'split_horizontal', label: '上下2分割', capacity: 2, cols: 1, rows: 2 },
  { value: 'grid_2x2', label: '4分割', capacity: 4, cols: 2, rows: 2 },
]

export function layoutOf(value) {
  return LAYOUTS.find((l) => l.value === value) || LAYOUTS[0]
}
