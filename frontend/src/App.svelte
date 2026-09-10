<script>
  import { onMount } from 'svelte'
  import { api, LAYOUTS, layoutOf } from './lib/api.js'
  import Terminal from './lib/Terminal.svelte'
  import GitMenu from './lib/GitMenu.svelte'
  import { neighborSlot } from './lib/paneNav.js'
  import { cycleWorkspaceId, workspaceIdAt } from './lib/workspaceNav.js'
  import { SHORTCUT_GROUPS, paneShortcutAction } from './lib/shortcuts.js'
  import { aggregateByWorkspace, connectAgentStatus, markSeen, nextDoneSet, withDone, STATE_LABEL, STATE_GLYPH } from './lib/agentStatus.js'
  import { collectActiveTerminals, gridDimensions } from './lib/activeTerminals.js'
  import { parseHash, buildHash, sidebarTasks, TASK_STATE_LABEL, agentsForTask, sourceOf } from './lib/tasks.js'
  import TaskList from './lib/tasks/TaskList.svelte'
  import TaskImport from './lib/tasks/TaskImport.svelte'
  import TaskDetail from './lib/tasks/TaskDetail.svelte'
  import SetupRun from './lib/tasks/SetupRun.svelte'
  import TaskSettings from './lib/tasks/TaskSettings.svelte'

  let workspaces = $state([])
  let rawAgentPanes = $state({}) // paneId → [{tool, state}]（サーバから push。working/blocked/idle/unknown）
  // 見ていないペインが working/blocked → idle に落ち着いたら「完了(未確認)」として
  // done を付ける。そのペインをアクティブにしたら外れる(herdr の done/seen と同じ考え方)。
  let donePaneIds = $state(new Set())
  const agentPanes = $derived(withDone(rawAgentPanes, donePaneIds))
  const agentByWs = $derived(aggregateByWorkspace(agentPanes, workspaces))
  let current = $state(null) // 選択中の WorkspaceDTO
  let openedPaneIds = $state(new Set()) // 現ワークスペースでライブセッションを持つ pane
  let livePaneIds = $state(new Set()) // 全ワークスペース横断のライブセッション

  // 表示モード: 'workspace' = 従来のレイアウト / 'active' = 全ワークスペース横断で
  // 起動中のターミナルだけを集約したビュー。
  let viewMode = $state(localStorage.getItem('mt.viewMode') === 'active' ? 'active' : 'workspace')
  let activeAgentOnly = $state(localStorage.getItem('mt.activeAgentOnly') === '1')
  const activeTerminals = $derived(
    collectActiveTerminals({ workspaces, livePaneIds, agentPanes, agentOnly: activeAgentOnly })
  )
  const activeGrid = $derived(gridDimensions(activeTerminals.length))
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
  let paneRemoteHost = $state('')
  let lastAutoDir = ''

  // ペイン毎の git 情報（paneId -> {isRepo, branch, dirty}）
  let paneGit = $state({})

  // git メニューを開いているペイン(null = 閉)
  let gitMenuPaneId = $state(null)

  function toggleGitMenu(paneId) {
    gitMenuPaneId = gitMenuPaneId === paneId ? null : paneId
  }

  // ペインタイトルインライン編集
  let editingTitlePaneId = $state(null)
  let titleDraft = $state('')

  // ペイン内容（タイトル・リポジトリ・作業ディレクトリ・起動コマンド）編集
  let editingPaneId = $state(null)
  let editTitle = $state('')
  let editRepoUrl = $state('')
  let editDir = $state('')
  let editCmds = $state('')
  let editAutoRun = $state(true)
  let editRemoteHost = $state('')
  let editLastAutoDir = ''

  // サイドバー折りたたみ
  let sidebarCollapsed = $state(localStorage.getItem('mt.sidebarCollapsed') === '1')

  // サイドバー各セクション(新規作成 / ワークスペース / アクティブ)の開閉。
  // どれかが長くなっても他が押し出されないよう、見出しクリックで畳める。
  let collapsedSections = $state(loadCollapsedSections())
  function loadCollapsedSections() {
    try {
      const v = JSON.parse(localStorage.getItem('mt.sidebarSections') || '{}')
      return v && typeof v === 'object' ? v : {}
    } catch {
      return {}
    }
  }
  function toggleSection(name) {
    collapsedSections = { ...collapsedSections, [name]: !collapsedSections[name] }
    localStorage.setItem('mt.sidebarSections', JSON.stringify(collapsedSections))
  }

  // 右上の ⚙ 設定メニュー(リモート設定 / ショートカット一覧 / タスク環境)
  let showSettingsMenu = $state(false)

  // ---- タスク管理(Jira 取り込み / 環境セットアップ) ----
  // 画面は URL ハッシュで決める(#tasks / #tasks/KEY / #tasks/KEY/setup / #settings/xxx / #ws)。
  // ハッシュが無ければ前回の画面(localStorage)。
  let route = $state(initialRoute())
  let tasks = $state([]) // /api/tasks の TaskDTO[]
  let tasksLoaded = $state(false)
  let templates = $state([])
  let baseClones = $state([])
  let taskSettings = $state(null) // /api/task-settings(ローカルタスクのステータス定義など)
  let jiraReady = $state(false) // Jira 接続が設定済みか(追加モーダルの既定タブ)
  const localStatuses = $derived(taskSettings?.localTask?.statuses || [])
  let currentTask = $state(null) // 詳細 / セットアップ画面で表示中のタスク
  let setupRunId = $state('') // セットアップ画面の run
  let showImport = $state(false)
  let lastTaskSync = $state(localStorage.getItem('mt.lastTaskSync') || '')
  const sidebarTaskItems = $derived(sidebarTasks(tasks))
  // ワークスペース画面のタスク帯(紐付くタスクがあるときだけ)
  const currentTaskStrip = $derived(current?.taskId ? tasks.find((t) => t.id === current.taskId) || null : null)

  function initialRoute() {
    if (location.hash && location.hash !== '#') return parseHash(location.hash)
    try {
      const saved = localStorage.getItem('mt.route')
      if (saved) return parseHash(saved)
    } catch {
      // 無視
    }
    return { screen: 'workspace' }
  }

  // navigate は画面を切り替える。ハッシュを書き換え、hashchange で route に反映される。
  function navigate(r) {
    const h = buildHash(r)
    if (location.hash === h) {
      route = r
      return
    }
    location.hash = h
  }

  function onHashChange() {
    route = parseHash(location.hash)
  }

  async function refreshTasks() {
    tasks = (await api.listTasks()) || []
    tasksLoaded = true
  }

  async function refreshTaskMeta() {
    const [tpls, bcs, st, jc] = await Promise.all([
      api.listTemplates(),
      api.listBaseClones(),
      api.getTaskSettings().catch(() => null),
      api.getJiraConfig().catch(() => null),
    ])
    templates = tpls || []
    baseClones = bcs || []
    if (st) taskSettings = st
    jiraReady = !!(jc && jc.hasToken && jc.baseUrl)
  }

  // 一覧のセレクトからローカルタスクのステータスを変える
  async function changeLocalStatus(t, statusId) {
    try {
      const updated = await api.patchTask(t.id, {
        local: {
          summary: t.jira?.summary || '',
          description: t.jira?.description || '',
          priority: t.jira?.priority || '',
          labels: t.local?.labels || [],
          links: t.local?.links || [],
          statusId,
        },
      })
      onTaskChanged(updated)
    } catch (e) {
      error = e.message
    }
  }

  // 手動作成の完了。セットアップ付きなら進捗画面へ、そうでなければ詳細へ
  async function onLocalCreated(res, setup) {
    showImport = false
    await refreshTasks().catch((e) => (error = e.message))
    const t = res?.task
    if (!t) return
    currentTask = t
    if (setup && res.runId) {
      setupRunId = res.runId
      navigate({ screen: 'setup', key: t.jiraKey })
    } else {
      navigate({ screen: 'task', key: t.jiraKey })
    }
  }

  // 詳細 / セットアップ画面のタスクをハッシュの番号から読み直す
  async function loadCurrentTask(key) {
    try {
      currentTask = await api.getTaskByKey(key)
      if (currentTask?.setupRunId) setupRunId = currentTask.setupRunId
    } catch (e) {
      error = e.message
      currentTask = null
    }
  }

  // 画面が変わったときに必要なデータを取り直す
  $effect(() => {
    const r = route
    try {
      localStorage.setItem('mt.route', buildHash(r))
    } catch {
      // 無視
    }
    if (r.screen === 'tasks') {
      refreshTasks().catch((e) => (error = e.message))
      refreshTaskMeta().catch(() => {})
    } else if (r.screen === 'task' || r.screen === 'setup') {
      loadCurrentTask(r.key)
      refreshTaskMeta().catch(() => {})
    } else if (r.screen === 'settings') {
      // 設定画面で変えたステータス定義などを一覧・詳細へ反映するため、離脱時に取り直す
      return () => refreshTaskMeta().catch(() => {})
    }
  })

  // タスクからワークスペースを開く
  async function openTaskWorkspace(t) {
    if (!t?.workspaceId) return
    await select(t.workspaceId)
  }

  // セットアップを開始して進捗画面へ
  async function startSetupFor(t) {
    try {
      const res = await api.startSetup(t.id)
      setupRunId = res.runId
      currentTask = t
      navigate({ screen: 'setup', key: t.jiraKey })
      refreshTasks().catch(() => {})
    } catch (e) {
      error = e.message
    }
  }

  function resumeSetup(t) {
    if (t.setupRunId) setupRunId = t.setupRunId
    currentTask = t
    navigate({ screen: 'setup', key: t.jiraKey })
  }

  async function onImported(res, setup) {
    showImport = false
    await refreshTasks().catch((e) => (error = e.message))
    const first = res?.tasks?.[0]
    if (setup && first && res.runIds?.[first.jiraKey]) {
      setupRunId = res.runIds[first.jiraKey]
      currentTask = first
      navigate({ screen: 'setup', key: first.jiraKey })
    }
  }

  async function syncAllTasks() {
    await guard(async () => {
      const res = await api.syncTasks()
      lastTaskSync = new Date().toISOString()
      localStorage.setItem('mt.lastTaskSync', lastTaskSync)
      await refreshTasks()
      if (res?.errors?.length) error = res.errors.join(' / ')
    })
  }

  async function deleteTaskWorkDir(t) {
    await guard(async () => {
      await api.deleteWorkDir(t.id)
      if (current?.id === t.workspaceId) current = null
      await refreshList()
      await refreshTasks()
      if (currentTask?.id === t.id) await loadCurrentTask(t.jiraKey)
    })
  }

  // 詳細画面での更新を一覧・帯にも反映する
  function onTaskChanged(updated) {
    if (currentTask?.id === updated.id) currentTask = updated
    tasks = tasks.map((t) => (t.id === updated.id ? updated : t))
    // Jira 紐付けで番号が変わったらハッシュも追従させる
    if (route.screen === 'task' && currentTask?.id === updated.id && route.key !== updated.jiraKey) {
      navigate({ screen: 'task', key: updated.jiraKey })
    }
  }

  function onSetupFinished() {
    refreshTasks().catch(() => {})
    refreshList().catch(() => {})
    if (route.screen === 'setup' && route.key) loadCurrentTask(route.key)
  }

  // ショートカット一覧モーダル
  let showShortcuts = $state(false)

  // リモート設定モーダル（自分の公開鍵表示・許可鍵の管理）
  let showRemoteSettings = $state(false)
  let remoteIdentity = $state(null) // {exists, publicKey?, fingerprint?}
  let authorizedKeys = $state([]) // [{key, comment, fingerprint}]
  let newAuthKey = $state('')
  let newAuthComment = $state('')
  let copiedPubKey = $state(false)
  let confirmingRegenerateKey = $state(false)
  let confirmingDeleteKey = $state(false)

  // 削除確認
  let confirmingDeleteId = $state(null)

  // アクティブペイン
  let activePaneId = $state(null)

  // コピーしたペイン設定。端末セッションや画面出力ではなく、次の新規ペインに
  // 必要な永続設定だけを保持する。
  let copiedPaneConfig = $state(null)

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
    if (route.screen !== 'workspace') navigate({ screen: 'workspace' })
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
  // 併せて全ワークスペース分を livePaneIds に保持する（アクティブビュー用）。
  async function syncLiveSessions() {
    const res = await api.listSessions()
    const live = new Set(res?.paneIds || [])
    livePaneIds = live
    if (!current) {
      openedPaneIds = new Set()
      return
    }
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
    paneRemoteHost = ''
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
      await api.addPane(current.id, dir, addingSlot, commands, paneTitle.trim(), paneRemoteHost.trim())
      addingSlot = null
      await reloadCurrent()
    })
  }

  function removePane(paneId) {
    guard(async () => {
      await api.removePane(current.id, paneId)
      openedPaneIds.delete(paneId)
      openedPaneIds = new Set(openedPaneIds)
      livePaneIds.delete(paneId)
      livePaneIds = new Set(livePaneIds)
      await reloadCurrent()
    })
  }

  function copyPaneConfig(pane) {
    copiedPaneConfig = {
      directory: pane.directory,
      title: pane.title || '',
      remoteHost: pane.remoteHost || '',
      // コピー元の編集で配列を変更しても影響しないように複製する。
      commands: (pane.commands || []).map((command) => ({ ...command })),
    }
  }

  function pastePaneConfig(slot) {
    if (!copiedPaneConfig) return
    guard(async () => {
      const result = await api.addPane(
        current.id,
        copiedPaneConfig.directory,
        slot,
        copiedPaneConfig.commands.map((command) => ({ ...command })),
        copiedPaneConfig.title,
        copiedPaneConfig.remoteHost
      )
      await reloadCurrent()
      activePaneId = result.paneId
    })
  }

  function openWorkspace() {
    guard(async () => {
      const res = await api.open(current.id)
      const ids = (res?.panes || []).map((p) => p.paneId)
      openedPaneIds = new Set(ids)
      livePaneIds = new Set([...livePaneIds, ...ids])
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

  // 全ワークスペース分のペイン情報とライブセッションを取り直す。アクティブビューは
  // 他ワークスペースのペインも並べるため、現ワークスペースだけでは足りない。
  function refreshActiveView() {
    return guard(async () => {
      await refreshList()
      await syncLiveSessions()
    })
  }

  function setViewMode(mode) {
    viewMode = mode
    localStorage.setItem('mt.viewMode', mode)
    if (route.screen !== 'workspace') navigate({ screen: 'workspace' })
    if (mode === 'active') refreshActiveView()
  }

  function toggleAgentOnly(value) {
    activeAgentOnly = value
    localStorage.setItem('mt.activeAgentOnly', value ? '1' : '0')
  }

  // アクティブビューのペインをワークスペース表示で開く（該当ワークスペースへ移動）。
  async function jumpToPane(t) {
    setViewMode('workspace')
    if (current?.id !== t.workspaceId) await select(t.workspaceId)
    activePaneId = t.paneId
  }

  function onKey(e) {
    // Cmd+/: ショートカット一覧の表示/非表示
    if (e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && e.key === '/') {
      e.preventDefault()
      e.stopPropagation()
      showShortcuts = !showShortcuts
      return
    }
    if (showShortcuts && e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      showShortcuts = false
      return
    }
    if (showRemoteSettings && e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      showRemoteSettings = false
      return
    }
    if (showImport && e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      showImport = false
      return
    }
    if (showSettingsMenu && e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      showSettingsMenu = false
      return
    }
    // Ctrl+Shift+A: アクティブビュー（起動中のターミナル集約）と通常表示を切替
    if (e.ctrlKey && e.shiftKey && !e.altKey && !e.metaKey && e.key?.toLowerCase() === 'a') {
      e.preventDefault()
      e.stopPropagation()
      setViewMode(viewMode === 'active' ? 'workspace' : 'active')
      return
    }
    // Cmd+1〜9: N 番目のワークスペースへ直接ジャンプ
    if (e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && e.key >= '1' && e.key <= '9') {
      const id = workspaceIdAt(workspaces, Number(e.key) - 1)
      if (id == null) return
      e.preventDefault()
      e.stopPropagation()
      if (id !== current?.id) select(id)
      return
    }
    // Ctrl+Alt+↑/↓: 前/次のワークスペースへ（端で巡回）
    if (e.ctrlKey && e.altKey && !e.shiftKey && !e.metaKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      const id = cycleWorkspaceId(workspaces, current?.id ?? null, e.key === 'ArrowDown' ? 1 : -1)
      if (id == null) return
      e.preventDefault()
      e.stopPropagation()
      if (id !== current?.id) select(id)
      return
    }
    // Ctrl+Shift+Z/F/V/G/B: アクティブペインの最大化 / Finder / VS Code / リモート / git メニュー
    const paneAction = paneShortcutAction(e)
    if (paneAction) {
      const pane = current?.panes?.find((p) => p.id === activePaneId) || current?.panes?.[0]
      if (!pane) return
      // リポジトリでないペインでは 🌐 ボタン・git バッジ非表示と同様に無視する
      if ((paneAction === 'github' || paneAction === 'gitmenu') && !paneGit[pane.id]?.isRepo) return
      e.preventDefault()
      e.stopPropagation()
      if (paneAction === 'maximize') toggleMaximize(maximized || pane.id)
      else if (paneAction === 'gitmenu') toggleGitMenu(pane.id)
      else openPaneIn(pane.id, paneAction)
      return
    }
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

  // ペイン内容の編集（タイトル・リポジトリ・作業ディレクトリ・起動コマンド）。
  // タイトルはセルヘッダのクリックでもインライン編集できる。
  function startEditPane(pane) {
    editingPaneId = pane.id
    editTitle = pane.title || ''
    editRepoUrl = ''
    editDir = pane.directory || ''
    editCmds = (pane.commands || []).map((c) => c.command).join('\n')
    editAutoRun = pane.commands?.length ? pane.commands.every((c) => c.autoRun) : true
    editRemoteHost = pane.remoteHost || ''
    editLastAutoDir = ''
  }
  function cancelEditPane() {
    editingPaneId = null
  }
  // 追加フォームと同じく、URL 入力に応じて clone 先を自動補完する。既存の
  // ディレクトリやユーザーが手で書き換えた値は上書きしない。
  function onEditRepoUrlInput() {
    const name = repoNameFromUrl(editRepoUrl)
    if (!name) return
    if (!editDir.trim() || editDir === editLastAutoDir) {
      editDir = `~/src/github/${name}`
      editLastAutoDir = editDir
    }
  }
  function submitEditPane(paneId) {
    if (!editDir.trim()) {
      error = '作業ディレクトリを入力してください'
      return
    }
    const repoUrl = editRepoUrl.trim()
    const commands = editCmds
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((command) => ({ command, autoRun: editAutoRun }))
    guard(async () => {
      let dir = editDir.trim()
      if (repoUrl) {
        const res = await api.cloneRepo(repoUrl, dir)
        dir = res.path
      }
      await api.setPaneTitle(current.id, paneId, editTitle.trim())
      await api.setPaneDirectory(current.id, paneId, dir)
      await api.setPaneCommands(current.id, paneId, commands)
      await api.setPaneRemoteHost(current.id, paneId, editRemoteHost.trim())
      editingPaneId = null
      await reloadCurrent()
    })
  }

  // リモート設定モーダルを開き、自分の鍵の状態と許可鍵リストを読み込む。
  function openRemoteSettings() {
    showRemoteSettings = true
    copiedPubKey = false
    confirmingRegenerateKey = false
    confirmingDeleteKey = false
    guard(async () => {
      remoteIdentity = await api.remoteIdentity()
      await reloadAuthorizedKeys()
    })
  }
  // この端末の鍵をユーザー操作で作成する（初回起動時の自動生成はしない）。
  function createIdentity() {
    guard(async () => {
      remoteIdentity = await api.createIdentity()
      copiedPubKey = false
    })
  }
  // 鍵を再作成する（公開鍵が変わるため、他端末での再登録が必要）。
  function regenerateIdentity() {
    confirmingRegenerateKey = false
    guard(async () => {
      remoteIdentity = await api.regenerateIdentity()
      copiedPubKey = false
    })
  }
  // 鍵を削除する（再作成するまでリモート接続は不可）。
  function deleteIdentity() {
    confirmingDeleteKey = false
    guard(async () => {
      await api.deleteIdentity()
      remoteIdentity = { exists: false }
      copiedPubKey = false
    })
  }
  async function reloadAuthorizedKeys() {
    const res = await api.listAuthorizedKeys()
    authorizedKeys = res?.keys || []
  }
  function addAuthorizedKey() {
    if (!newAuthKey.trim()) {
      error = '公開鍵を入力してください'
      return
    }
    guard(async () => {
      await api.addAuthorizedKey(newAuthKey.trim(), newAuthComment.trim())
      newAuthKey = ''
      newAuthComment = ''
      await reloadAuthorizedKeys()
    })
  }
  function removeAuthorizedKey(key) {
    guard(async () => {
      await api.removeAuthorizedKey(key)
      await reloadAuthorizedKeys()
    })
  }
  async function copyPublicKey() {
    if (!remoteIdentity?.publicKey) return
    try {
      await navigator.clipboard.writeText(remoteIdentity.publicKey)
      copiedPubKey = true
      setTimeout(() => (copiedPubKey = false), 1500)
    } catch {
      // クリップボード不可の環境ではテキストを選択してもらう
    }
  }

  // ペインの作業ディレクトリを Finder / VS Code / ブラウザ / 別プロセスのターミナルで開く（バックエンド経由）。
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
    window.addEventListener('hashchange', onHashChange)
    refreshTasks().catch(() => {}) // サイドバーのタスク欄用(タスク機能が無効なら空のまま)
    const stopAgentStatus = connectAgentStatus((p) => {
      donePaneIds = nextDoneSet(donePaneIds, rawAgentPanes, p, activePaneId)
      rawAgentPanes = p
    })
    return () => {
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('hashchange', onHashChange)
      stopAgentStatus()
    }
  })

  // ペインをアクティブにしたら、その「完了(未確認)」印を消す。
  $effect(() => {
    donePaneIds = markSeen(donePaneIds, activePaneId)
  })

  // サイドバーのアクティブ一覧(と集約ビュー)は他ワークスペースで起動/終了したペインも
  // 並べるため、一覧とセッションを定期的に取り直す（エージェント状態は SSE で届くので対象外）。
  // タブが裏に回っている間は止める。
  $effect(() => {
    // 前回の取得が 3 秒以内に終わらないときは、リクエストの重複と
    // 到着順の入れ替わりによる状態の巻き戻りを避けるためにこの周期を飛ばす。
    let inFlight = false
    const timer = setInterval(async () => {
      if (inFlight || document.hidden) return
      inFlight = true
      try {
        await refreshList()
        await syncLiveSessions()
        if (tasksLoaded) await refreshTasks()
      } catch {
        // サーバ再起動中など。次回の周期に任せる。
      } finally {
        inFlight = false
      }
    }, 3000)
    return () => clearInterval(timer)
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

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="app"
  class:sidebar-collapsed={sidebarCollapsed}
  onclick={(e) => {
    // ⚙ メニューの外をクリックしたら閉じる
    if (showSettingsMenu && !e.target.closest?.('.settings-menu-wrap')) showSettingsMenu = false
  }}
>
  <header class="appbar">
    <button
      class="icon sidebar-toggle"
      onclick={toggleSidebar}
      aria-label="サイドバー切替"
      aria-expanded={!sidebarCollapsed}
    >☰</button>
    <h1>multi-terminals</h1>
    <span class="spacer"></span>
    <button
      class="appbar-btn"
      title="ショートカット一覧 (⌘/)"
      aria-label="ショートカット一覧"
      onclick={() => (showShortcuts = true)}
    ><kbd>⌘</kbd><kbd>/</kbd></button>
    <div class="settings-menu-wrap">
      <button
        class="appbar-btn"
        class:open={showSettingsMenu}
        aria-haspopup="menu"
        aria-expanded={showSettingsMenu}
        title="設定"
        onclick={() => (showSettingsMenu = !showSettingsMenu)}
      >⚙ 設定 ▾</button>
      {#if showSettingsMenu}
        <div class="settings-menu" role="menu" aria-label="設定">
          <div class="grp">タスク環境</div>
          <button role="menuitem" onclick={() => { showSettingsMenu = false; navigate({ screen: 'settings', panel: 'templates' }) }}><span class="ic">🧩</span>環境テンプレート</button>
          <button role="menuitem" onclick={() => { showSettingsMenu = false; navigate({ screen: 'settings', panel: 'base' }) }}><span class="ic">⧉</span>ベースクローン</button>
          <button role="menuitem" onclick={() => { showSettingsMenu = false; navigate({ screen: 'settings', panel: 'jira' }) }}><span class="ic">🔗</span>Jira 接続</button>
          <button role="menuitem" onclick={() => { showSettingsMenu = false; navigate({ screen: 'settings', panel: 'local' }) }}><span class="ic">📝</span>ローカルタスク</button>
          <button role="menuitem" onclick={() => { showSettingsMenu = false; navigate({ screen: 'settings', panel: 'general' }) }}><span class="ic">⚙</span>セットアップ設定</button>
          <hr />
          <div class="grp">このアプリ</div>
          <button
            role="menuitem"
            onclick={() => {
              showSettingsMenu = false
              openRemoteSettings()
            }}
          ><span class="ic">🔑</span>リモート設定</button>
          <button
            role="menuitem"
            onclick={() => {
              showSettingsMenu = false
              showShortcuts = true
            }}
          ><span class="ic">⌨</span>ショートカット一覧<span class="hint">⌘/</span></button>
        </div>
      {/if}
    </div>
  </header>

  <aside class="sidebar">
    <!-- 進行中(作業中 / 準備中 / 環境あり)のタスクだけ最大 5 件。全件はメインの一覧 -->
    <section class="list task-list" class:collapsed={collapsedSections.tasks}>
      <h2>
        <button class="sec-toggle" aria-expanded={!collapsedSections.tasks} onclick={() => toggleSection('tasks')}>
          <span class="chev">▾</span>タスク<span class="sub">· 進行中</span>
        </button>
        <span class="count">{sidebarTaskItems.length} / {tasks.length}</span>
      </h2>
      <div class="sec-body">
        {#if tasks.length === 0}
          <p class="muted">まだありません</p>
        {:else if sidebarTaskItems.length === 0}
          <p class="muted">進行中のタスクはありません</p>
        {:else}
          <ul>
            {#each sidebarTaskItems as t (t.id)}
              {@const agents = agentsForTask(t, workspaces, agentPanes)}
              <li>
                <button
                  class="task-select"
                  class:active={route.screen !== 'workspace' ? currentTask?.id === t.id : current?.taskId === t.id}
                  title="{t.jiraKey} {t.jira?.summary || ''}"
                  onclick={() => (t.workspaceId && t.effectiveState !== 'preparing' ? openTaskWorkspace(t) : navigate({ screen: 'task', key: t.jiraKey }))}
                >
                  <span class="act-dot {t.effectiveState}"></span>
                  <span class="task-key" class:local={sourceOf(t) === 'local'}>{t.jiraKey}</span>
                  {#if agents.length > 0}
                    {#each agents as a (a.tool)}
                      <span class="agent-badge">
                        {a.tool}
                        {#if a.blocked}<span class="agent-st blocked">{STATE_GLYPH.blocked}{a.blocked}</span>{/if}
                        {#if a.working}<span class="agent-st working">{STATE_GLYPH.working}{a.working}</span>{/if}
                        {#if a.done}<span class="agent-st done">{STATE_GLYPH.done}{a.done}</span>{/if}
                      </span>
                    {/each}
                  {:else}
                    <span class="badge">{TASK_STATE_LABEL[t.effectiveState] || t.effectiveState}</span>
                  {/if}
                  <span class="task-sum">{t.jira?.summary || ''}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <button class="more" onclick={() => navigate({ screen: 'tasks' })}>📋 すべてのタスク <span class="n">{tasks.length} →</span></button>
      </div>
    </section>

    <section class="create" class:collapsed={collapsedSections.create}>
      <h2>
        <button class="sec-toggle" aria-expanded={!collapsedSections.create} onclick={() => toggleSection('create')}>
          <span class="chev">▾</span>新規ワークスペース
        </button>
      </h2>
      <div class="sec-body">
        <input placeholder="名前" bind:value={newName} />
        <select bind:value={newLayout}>
          {#each LAYOUTS as l}
            <option value={l.value}>{l.label}</option>
          {/each}
        </select>
        <button onclick={createWorkspace} disabled={busy}>作成</button>
      </div>
    </section>

    <section class="list" class:collapsed={collapsedSections.workspaces}>
      <h2>
        <button class="sec-toggle" aria-expanded={!collapsedSections.workspaces} onclick={() => toggleSection('workspaces')}>
          <span class="chev">▾</span>ワークスペース
        </button>
        <span class="count">{workspaces.length}</span>
      </h2>
      {#if workspaces.length === 0}
        <p class="muted sec-body">まだありません</p>
      {/if}
      <ul class="sec-body">
        {#each workspaces as w, i}
          <li>
            <button class="ws-select" class:active={current?.id === w.id} onclick={() => select(w.id)}>
              {#if i < 9}<span class="ws-key" title="⌘{i + 1} で切替">⌘{i + 1}</span>{/if}
              <span class="name">{w.name}</span>
              {#if agentByWs.get(w.id)}
                {#each agentByWs.get(w.id) as a (a.tool)}
                  <span
                    class="agent-badge"
                    title="{a.tool}: 応答待ち {a.blocked} / 処理中 {a.working} / 完了(未確認) {a.done} / 入力待ち {a.idle} / 不明 {a.unknown}"
                  >
                    {a.tool}
                    {#if a.blocked > 0}<span class="agent-st blocked">{STATE_GLYPH.blocked}{a.blocked}</span>{/if}
                    {#if a.working > 0}<span class="agent-st working">{STATE_GLYPH.working}{a.working}</span>{/if}
                    {#if a.done > 0}<span class="agent-st done">{STATE_GLYPH.done}{a.done}</span>{/if}
                    {#if a.idle > 0}<span class="agent-st idle">{STATE_GLYPH.idle}{a.idle}</span>{/if}
                    {#if a.unknown > 0}<span class="agent-st unknown">{STATE_GLYPH.unknown}{a.unknown}</span>{/if}
                  </span>
                {/each}
              {/if}
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

    <!-- 起動中のターミナルをワークスペース横断で常時一覧表示（タブ切替ではなくワークスペースの下に置く） -->
    <section class="list active-list" class:collapsed={collapsedSections.active}>
      <h2>
        <button class="sec-toggle" aria-expanded={!collapsedSections.active} onclick={() => toggleSection('active')}>
          <span class="chev">▾</span>⚡ アクティブ
        </button>
        <span class="count">{activeTerminals.length}</span>
      </h2>
      <div class="sec-body">
        {#if activeTerminals.length === 0}
          <p class="muted">
            {activeAgentOnly ? 'claude / codex 稼働中のターミナルはありません' : '起動中のターミナルはありません'}
          </p>
        {:else}
          <ul>
            {#each activeTerminals as t (t.paneId)}
              <li>
                <button
                  class="act-select"
                  class:active={t.paneId === activePaneId && viewMode === 'workspace' && current?.id === t.workspaceId}
                  title="{t.workspaceName} / {t.directory}{t.remoteHost ? ` (リモート: ${t.remoteHost})` : ''}"
                  onclick={() => jumpToPane(t)}
                >
                  <span class="act-dot {t.status}"></span>
                  <span class="act-title">{t.title || t.directory}</span>
                  {#if t.agents.length > 0}
                    {#each t.agents as a (a.tool)}
                      <span class="agent-badge" title="{a.tool}: {STATE_LABEL[a.state] || a.state}">
                        {a.tool}
                        <span class="agent-st {a.state}">{STATE_GLYPH[a.state] || '?'}</span>
                      </span>
                    {/each}
                  {:else if t.remoteHost}
                    <span class="act-tag">🖥 {t.remoteHost}</span>
                  {/if}
                  <span class="act-where">{t.workspaceName} · pane {t.slot}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <div class="act-foot">
          <label class="inline-check">
            <input
              type="checkbox"
              checked={activeAgentOnly}
              onchange={(e) => toggleAgentOnly(e.currentTarget.checked)}
            />
            エージェントのみ
          </label>
          <span class="spacer"></span>
          <button
            class="icon"
            class:active={viewMode === 'active'}
            title="起動中のターミナルだけを 1 画面に集約 (Ctrl+Shift+A)"
            aria-pressed={viewMode === 'active'}
            onclick={() => setViewMode(viewMode === 'active' ? 'workspace' : 'active')}
          >⤢ 集約表示</button>
        </div>
      </div>
    </section>
  </aside>

  <main class="workspace">
    {#if error}
      <div class="error" role="alert">{error}</div>
    {/if}

    {#if route.screen === 'tasks'}
      <TaskList
        {tasks}
        {workspaces}
        {agentPanes}
        {busy}
        lastSync={lastTaskSync}
        {localStatuses}
        onStatusChange={changeLocalStatus}
        onOpen={openTaskWorkspace}
        onDetail={(t) => navigate({ screen: 'task', key: t.jiraKey })}
        onImport={() => (showImport = true)}
        onSync={syncAllTasks}
        onSetup={startSetupFor}
        onResume={resumeSetup}
        onDeleteWorkDir={deleteTaskWorkDir}
      />
    {:else if route.screen === 'task'}
      {#if currentTask}
        <TaskDetail
          task={currentTask}
          {templates}
          {baseClones}
          {localStatuses}
          {busy}
          onBack={() => navigate({ screen: 'tasks' })}
          onOpenWorkspace={openTaskWorkspace}
          onSetup={startSetupFor}
          onResume={resumeSetup}
          onChanged={onTaskChanged}
          onDeleted={() => { currentTask = null; refreshTasks().catch(() => {}); navigate({ screen: 'tasks' }) }}
          onDeleteWorkDir={deleteTaskWorkDir}
          onError={(m) => (error = m)}
        />
      {:else}
        <div class="toolbar"><button class="icon" onclick={() => navigate({ screen: 'tasks' })}>← タスク</button><strong>{route.key}</strong></div>
        <div class="empty">タスクが見つかりません</div>
      {/if}
    {:else if route.screen === 'setup'}
      {#if setupRunId}
        <SetupRun
          runId={setupRunId}
          task={currentTask}
          onBack={() => navigate(currentTask ? { screen: 'task', key: currentTask.jiraKey } : { screen: 'tasks' })}
          onOpenWorkspace={(id) => select(id)}
          onFinished={onSetupFinished}
          onError={(m) => (error = m)}
        />
      {:else}
        <div class="toolbar"><button class="icon" onclick={() => navigate({ screen: 'tasks' })}>← タスク</button><strong>{route.key}</strong></div>
        <div class="empty">セットアップの実行記録がありません</div>
      {/if}
    {:else if route.screen === 'settings'}
      <div class="toolbar">
        <strong>設定</strong>
        <span class="spacer"></span>
        <button class="icon" onclick={() => navigate(current ? { screen: 'workspace' } : { screen: 'tasks' })}>✕ 閉じる</button>
      </div>
      <TaskSettings panel={route.panel} onNavigate={(p) => navigate({ screen: 'settings', panel: p })} onError={(m) => (error = m)} />
    {:else if viewMode === 'active'}
      <!-- 全ワークスペース横断で、起動中のターミナルだけを 1 画面に集約する -->
      <div class="toolbar">
        <strong>アクティブなターミナル</strong>
        <label class="inline-check">
          <input
            type="checkbox"
            checked={activeAgentOnly}
            onchange={(e) => toggleAgentOnly(e.currentTarget.checked)}
          />
          エージェント稼働中のみ
        </label>
        <span class="spacer"></span>
        <span class="muted">{activeTerminals.length} 件</span>
        <button onclick={refreshActiveView} disabled={busy}>⟳ 更新</button>
      </div>

      {#if activeTerminals.length === 0}
        <div class="empty">
          {activeAgentOnly
            ? 'claude / codex が稼働中のターミナルはありません'
            : '起動中のターミナルはありません（ワークスペースを開くとここに集まります）'}
        </div>
      {:else}
        <div
          class="grid active-grid"
          style="grid-template-columns: repeat({activeGrid.cols}, 1fr);"
        >
          {#each activeTerminals as t (t.paneId)}
            <div class="cell" class:active-cell={t.paneId === activePaneId}>
              <div class="cell-head">
                <span class="ws-tag" title="ワークスペース: {t.workspaceName}">{t.workspaceName}</span>
                <span class="dir" title={t.directory}>{t.title || t.directory}</span>
                {#if t.remoteHost}
                  <span class="remote-badge" title="リモート実行: {t.remoteHost}">🖥 {t.remoteHost}</span>
                {/if}
                {#each t.agents as a (a.tool)}
                  <span
                    class="agent-badge"
                    title="{a.tool}: {STATE_LABEL[a.state] || a.state}{a.rule ? ` (${a.rule})` : ''}"
                  >
                    {a.tool}
                    <span class="agent-st {a.state}">{STATE_GLYPH[a.state] || '?'}</span>
                  </span>
                {/each}
                <span class="cell-actions">
                  <button
                    type="button"
                    class="icon"
                    title="このペインをワークスペース表示で開く"
                    aria-label="{t.title || t.directory} をワークスペース表示で開く"
                    onclick={() => jumpToPane(t)}
                  >↗</button>
                </span>
              </div>
              <div class="cell-body">
                <Terminal
                  paneId={t.paneId}
                  active={t.paneId === activePaneId}
                  onActivate={() => (activePaneId = t.paneId)}
                />
              </div>
            </div>
          {/each}
        </div>
      {/if}
    {:else if !current}
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
      {#if currentTaskStrip}
        <div class="task-strip">
          <span class="key" class:local={sourceOf(currentTaskStrip) === 'local'}>{currentTaskStrip.jiraKey}</span>
          <span class="sum">{currentTaskStrip.jira?.summary || ''}</span>
          <span class="pill">{currentTaskStrip.jira?.status || '—'}</span>
          {#if currentTaskStrip.env?.branch}<span class="muted">· <code>{currentTaskStrip.env.branch}</code></span>{/if}
          <span class="spacer"></span>
          <button class="icon" onclick={() => navigate({ screen: 'task', key: currentTaskStrip.jiraKey })}>タスク詳細</button>
          {#if currentTaskStrip.jira?.url}
            <a class="icon-link" href={currentTaskStrip.jira.url} target="_blank" rel="noopener">Jira で開く ↗</a>
          {:else if currentTaskStrip.local?.links?.[0]}
            <a class="icon-link" href={currentTaskStrip.local.links[0]} target="_blank" rel="noopener">🔗 リンクを開く ↗</a>
          {/if}
        </div>
      {/if}

      <div
        class="grid"
        style="grid-template-columns: repeat({maximized ? 1 : layout.cols}, 1fr); grid-template-rows: repeat({maximized ? 1 : layout.rows}, 1fr);"
      >
        {#each slots as cell (cell.slot)}
          <div class="cell" class:active-cell={cell.pane && cell.pane.id === activePaneId}>
            {#if cell.pane}
              <div class="cell-head">
                <div class="cell-title-row">
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
                {#if cell.pane.remoteHost}
                  <span class="remote-badge" title="リモート実行: {cell.pane.remoteHost}">🖥 {cell.pane.remoteHost}</span>
                {/if}
                {#if paneGit[cell.pane.id]?.isRepo}
                  <span class="git-wrap">
                    <span
                      class="git-badge"
                      class:dirty={paneGit[cell.pane.id].dirty}
                      title={paneGit[cell.pane.id].dirty ? '未コミットの変更あり(クリックで git メニュー)' : 'クリックで git メニュー'}
                      role="button"
                      tabindex="0"
                      onclick={(e) => { e.stopPropagation(); toggleGitMenu(cell.pane.id) }}
                      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleGitMenu(cell.pane.id) } }}
                    >⎇ {paneGit[cell.pane.id].branch}{paneGit[cell.pane.id].dirty ? '*' : ''}</span>
                    {#if gitMenuPaneId === cell.pane.id}
                      <GitMenu
                        workspaceId={current.id}
                        paneId={cell.pane.id}
                        onClose={() => (gitMenuPaneId = null)}
                        onChanged={refreshGitInfo}
                      />
                    {/if}
                  </span>
                {/if}
                </div>
                <div class="cell-actions">
                  <button class="icon" title="Finderで開く" onclick={() => openPaneIn(cell.pane.id, 'finder')}>📁</button>
                  <button class="icon" title="VSCodeで開く" onclick={() => openPaneIn(cell.pane.id, 'vscode')}>{'</>'}</button>
                  {#if paneGit[cell.pane.id]?.isRepo}
                    <button class="icon" title="リポジトリをブラウザで開く" onclick={() => openPaneIn(cell.pane.id, 'github')}>🌐</button>
                  {/if}
                  <button class="icon" title="別プロセスのターミナルで開く" onclick={() => openPaneIn(cell.pane.id, 'terminal')}>⧉</button>
                  <button class="icon" title="編集" onclick={() => startEditPane(cell.pane)}>⚙</button>
                  <button class="icon" title="このペインの設定をコピー" onclick={() => copyPaneConfig(cell.pane)}>📋</button>
                  <button class="icon" title="最大化/戻す" onclick={() => toggleMaximize(cell.pane.id)}>⤢</button>
                  <button class="icon" title="閉じる" onclick={() => removePane(cell.pane.id)}>✕</button>
                </div>
              </div>
              <div class="cell-body">
                {#if editingPaneId === cell.pane.id}
                  <div class="add-form">
                    <h3>ペインを編集</h3>
                    <label>タイトル（任意）
                      <input placeholder="例: API サーバー" bind:value={editTitle} use:focusOnMount />
                    </label>
                    <label>リポジトリ URL（任意・未 clone なら自動 clone）
                      <input
                        placeholder="https://github.com/user/repo.git"
                        bind:value={editRepoUrl}
                        oninput={onEditRepoUrlInput}
                      />
                    </label>
                    <label>{editRepoUrl.trim() ? 'clone 先ディレクトリ' : '作業ディレクトリ'}
                      <input placeholder="/path/to/project" bind:value={editDir} />
                    </label>
                    <label>リモートホスト（任意・空でローカル実行）
                      <input placeholder="例: 192.168.1.10:8080 または ssh://user@host" bind:value={editRemoteHost} />
                      <small class="muted">multi-terminals 同士は host:port（TLS は https://）。既存 SSH サーバへは ssh://user@host[:port]（~/.ssh の鍵で認証）。</small>
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
                <label>リポジトリ URL（任意・未 clone なら自動 clone）
                  <input
                    placeholder="https://github.com/user/repo.git"
                    bind:value={paneRepoUrl}
                    oninput={onRepoUrlInput}
                  />
                </label>
                <label>{paneRepoUrl.trim() ? 'clone 先ディレクトリ' : '作業ディレクトリ'}
                  <input placeholder="/path/to/project" bind:value={paneDir} />
                </label>
                <label>リモートホスト（任意・空でローカル実行）
                  <input placeholder="例: 192.168.1.10:8080 または ssh://user@host" bind:value={paneRemoteHost} />
                  <small class="muted">multi-terminals 同士は host:port（TLS は https://）。既存 SSH サーバへは ssh://user@host[:port]（~/.ssh の鍵で認証）。</small>
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
              <div class="empty-slot-actions">
                <button class="empty-slot" onclick={() => startAddPane(cell.slot)}>＋ ペインを追加</button>
                {#if copiedPaneConfig}
                  <button class="empty-slot" onclick={() => pastePaneConfig(cell.slot)} disabled={busy}>
                    ⧉ 設定を貼り付けて複製
                  </button>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </main>

  {#if showImport}
    <TaskImport {templates} {jiraReady} {localStatuses} onClose={() => (showImport = false)} onImported={onImported} onCreated={onLocalCreated} onError={(m) => (error = m)} />
  {/if}

  {#if showRemoteSettings}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="shortcuts-overlay" role="presentation" onclick={() => (showRemoteSettings = false)}>
      <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
      <div
        class="shortcuts-modal remote-modal"
        role="dialog"
        aria-label="リモート設定"
        tabindex="-1"
        onclick={(e) => e.stopPropagation()}
      >
        <div class="shortcuts-head">
          <h2>🔑 リモート設定</h2>
          <button class="icon" title="閉じる" onclick={() => (showRemoteSettings = false)}>✕</button>
        </div>

        <h3>この端末の鍵</h3>
        {#if remoteIdentity && remoteIdentity.exists}
          <p class="muted remote-note">
            他の端末で実行したいときは、この公開鍵を<strong>実行側（待ち受け側）</strong>の「許可された鍵」に追加してください。
          </p>
          <div class="pubkey-row">
            <input class="pubkey" readonly value={remoteIdentity.publicKey} onfocus={(e) => e.currentTarget.select()} />
            <button onclick={copyPublicKey}>{copiedPubKey ? '✓ コピー済み' : 'コピー'}</button>
          </div>
          <p class="muted remote-note">フィンガープリント: <code>{remoteIdentity.fingerprint}</code></p>
          <div class="key-actions">
            {#if confirmingRegenerateKey}
              <span class="muted">再作成すると公開鍵が変わり、他端末の登録がやり直しになります。</span>
              <button class="danger" onclick={regenerateIdentity} disabled={busy}>再作成する</button>
              <button class="icon" onclick={() => (confirmingRegenerateKey = false)}>取消</button>
            {:else if confirmingDeleteKey}
              <span class="muted">削除すると他の multi-terminals へ接続できなくなります（再作成で復活）。</span>
              <button class="danger" onclick={deleteIdentity} disabled={busy}>削除する</button>
              <button class="icon" onclick={() => (confirmingDeleteKey = false)}>取消</button>
            {:else}
              <button onclick={() => (confirmingRegenerateKey = true)} disabled={busy}>再作成</button>
              <button class="danger" onclick={() => (confirmingDeleteKey = true)} disabled={busy}>削除</button>
            {/if}
          </div>
        {:else if remoteIdentity}
          <p class="muted remote-note">
            この端末にはまだ鍵がありません。他の multi-terminals へ<strong>接続する側</strong>として使うには鍵が必要です（自動生成はされません）。作成後、公開鍵を実行側の「許可された鍵」に追加してください。<br />
            ※ <code>ssh://</code> でのリモート実行にはこの鍵は不要です。
          </p>
          <button class="primary" onclick={createIdentity} disabled={busy}>この端末の鍵を作成</button>
        {/if}

        <h3>許可された鍵（この端末での実行を許可）</h3>
        <p class="muted remote-note">
          1つ以上許可すると、この端末はリモート実行の接続を受け付けます。空の間は待ち受け無効です。
        </p>
        {#if authorizedKeys.length === 0}
          <p class="muted">許可された鍵はありません（待ち受け無効）</p>
        {:else}
          <ul class="auth-keys">
            {#each authorizedKeys as k (k.key)}
              <li>
                <span class="auth-key-comment">{k.comment || '（コメントなし）'}</span>
                <code class="auth-key-fp" title={k.key}>{k.fingerprint}</code>
                <button class="icon danger" title="削除" onclick={() => removeAuthorizedKey(k.key)}>✕</button>
              </li>
            {/each}
          </ul>
        {/if}
        <div class="add-key">
          <input placeholder="ed25519:… （相手端末の公開鍵）" bind:value={newAuthKey} />
          <input class="key-comment" placeholder="コメント（例: ノートPC）" bind:value={newAuthComment} />
          <button class="primary" onclick={addAuthorizedKey} disabled={busy}>追加</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showShortcuts}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="shortcuts-overlay" role="presentation" onclick={() => (showShortcuts = false)}>
      <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
      <div
        class="shortcuts-modal"
        role="dialog"
        aria-label="キーボードショートカット"
        tabindex="-1"
        onclick={(e) => e.stopPropagation()}
      >
        <div class="shortcuts-head">
          <h2>キーボードショートカット</h2>
          <button class="icon" title="閉じる" onclick={() => (showShortcuts = false)}>✕</button>
        </div>
        {#each SHORTCUT_GROUPS as group}
          <h3>{group.label}</h3>
          <ul>
            {#each group.items as item}
              <li>
                <span class="keys">
                  {#each item.keys as k, i}
                    {#if i > 0}<span class="plus">+</span>{/if}<kbd>{k}</kbd>
                  {/each}
                </span>
                <span class="desc">{item.desc}</span>
              </li>
            {/each}
          </ul>
        {/each}
      </div>
    </div>
  {/if}
</div>
