/**
 * アプリ全体のキーボードショートカット定義（単一情報源）。
 * ヘルプモーダルの表示に使う。実際のハンドリングは App.svelte の onKey。
 * keys は <kbd> で 1 つずつ描画するトークン列。
 */
export const SHORTCUT_GROUPS = [
  {
    label: 'ワークスペース',
    items: [
      { keys: ['Ctrl', 'Alt', '↑'], desc: '前のワークスペースへ（端で巡回）' },
      { keys: ['Ctrl', 'Alt', '↓'], desc: '次のワークスペースへ（端で巡回）' },
      { keys: ['⌘', '1〜9'], desc: 'N 番目のワークスペースへジャンプ' },
    ],
  },
  {
    label: 'ペイン',
    items: [
      { keys: ['Ctrl', 'Shift', '← → ↑ ↓'], desc: '隣のペインへフォーカス移動' },
      { keys: ['Ctrl', 'Shift', 'Z'], desc: 'アクティブペインを最大化 / 元に戻す' },
      { keys: ['Ctrl', 'Shift', 'F'], desc: 'アクティブペインを Finder で開く' },
      { keys: ['Ctrl', 'Shift', 'V'], desc: 'アクティブペインを VS Code で開く' },
      { keys: ['Ctrl', 'Shift', 'G'], desc: 'リモート (GitHub) を開く（リポジトリのみ）' },
      { keys: ['Ctrl', 'Shift', 'B'], desc: 'git メニューを開閉（リポジトリのみ。↑↓+Enter=ブランチ切替。P/U/F=pull/push/fetch ※絞り込み入力中は絞り込み文字として扱う）' },
    ],
  },
  {
    label: 'その他',
    items: [
      { keys: ['⌘', '/'], desc: 'ショートカット一覧を表示 / 非表示' },
      { keys: ['Esc'], desc: 'ショートカット一覧を閉じる' },
    ],
  },
]

/** Ctrl+Shift+英字のペイン操作アクション名。対象外のイベントは null。 */
const PANE_ACTIONS = { z: 'maximize', f: 'finder', v: 'vscode', g: 'github', b: 'gitmenu' }

/**
 * キーイベントをペイン操作アクションへ対応付ける pure 関数。
 * @param {{ctrlKey: boolean, shiftKey: boolean, altKey: boolean, metaKey: boolean, key: string}} e
 * @returns {'maximize'|'finder'|'vscode'|'github'|'gitmenu'|null}
 */
export function paneShortcutAction(e) {
  if (!e.ctrlKey || !e.shiftKey || e.altKey || e.metaKey) return null
  return PANE_ACTIONS[e.key?.toLowerCase()] ?? null
}
