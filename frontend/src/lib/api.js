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
  paneGitBranches: (id, paneId) => req('GET', `/api/workspaces/${id}/panes/${paneId}/git/branches`),
  paneGitCheckout: (id, paneId, branch) =>
    req('POST', `/api/workspaces/${id}/panes/${paneId}/git/checkout`, { branch }),
  paneGitOp: (id, paneId, op) => req('POST', `/api/workspaces/${id}/panes/${paneId}/git/${op}`),
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

  // ---- タスク管理（Jira 取り込み / 環境セットアップ） ----
  listTasks: () => req('GET', '/api/tasks'),
  createLocalTask: (input) => req('POST', '/api/tasks', input),
  nextLocalKey: () => req('GET', '/api/tasks/next-key'),
  linkJira: (id, key) => req('POST', `/api/tasks/${id}/link-jira`, { key }),
  getTask: (id) => req('GET', `/api/tasks/${id}`),
  getTaskByKey: (key) => req('GET', `/api/tasks/by-key/${encodeURIComponent(key)}`),
  previewTasks: (keys, templateId) => req('POST', '/api/tasks/preview', { keys, templateId: templateId || '' }),
  importTasks: (keys, templateId, setup) =>
    req('POST', '/api/tasks/import', { keys, templateId: templateId || '', setup: !!setup }),
  syncTasks: () => req('POST', '/api/tasks/sync'),
  syncTask: (id) => req('POST', `/api/tasks/${id}/sync`),
  patchTask: (id, patch) => req('PATCH', `/api/tasks/${id}`, patch),
  deleteTask: (id) => req('DELETE', `/api/tasks/${id}`),
  startSetup: (id) => req('POST', `/api/tasks/${id}/setup`),
  deleteWorkDir: (id) => req('POST', `/api/tasks/${id}/workdir/delete`),
  // セットアップ実行
  getSetupRun: (id) => req('GET', `/api/setup-runs/${id}`),
  retrySetupRun: (id) => req('POST', `/api/setup-runs/${id}/retry`),
  skipSetupStep: (id) => req('POST', `/api/setup-runs/${id}/skip`),
  abortSetupRun: (id) => req('POST', `/api/setup-runs/${id}/abort`),
  // 環境テンプレート
  listTemplates: () => req('GET', '/api/templates'),
  createTemplate: (input) => req('POST', '/api/templates', input),
  updateTemplate: (id, input) => req('PUT', `/api/templates/${id}`, input),
  deleteTemplate: (id) => req('DELETE', `/api/templates/${id}`),
  // ベースクローン
  listBaseClones: () => req('GET', '/api/base-clones'),
  createBaseClone: (input) => req('POST', '/api/base-clones', input),
  updateBaseClone: (id) => req('POST', `/api/base-clones/${id}/update`),
  updateAllBaseClones: () => req('POST', '/api/base-clones/update'),
  deleteBaseClone: (id, removeFiles) => req('DELETE', `/api/base-clones/${id}${removeFiles ? '?files=1' : ''}`),
  // 設定
  getTaskSettings: () => req('GET', '/api/task-settings'),
  putTaskSettings: (s) => req('PUT', '/api/task-settings', s),
  getJiraConfig: () => req('GET', '/api/jira/config'),
  putJiraConfig: (input) => req('PUT', '/api/jira/config', input),
  testJira: () => req('POST', '/api/jira/test'),
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
