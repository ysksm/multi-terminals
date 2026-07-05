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
    items: [{ keys: ['Ctrl', 'Shift', '← → ↑ ↓'], desc: '隣のペインへフォーカス移動' }],
  },
  {
    label: 'その他',
    items: [
      { keys: ['⌘', '/'], desc: 'ショートカット一覧を表示 / 非表示' },
      { keys: ['Esc'], desc: 'ショートカット一覧を閉じる' },
    ],
  },
]
