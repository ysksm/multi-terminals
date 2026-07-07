/**
 * ブランチ一覧を名前の部分一致で絞り込む pure 関数(大文字小文字を無視)。
 * クエリが空(空白のみ含む)なら元の配列をそのまま返す。
 * @param {Array<{name: string}>} branches
 * @param {string} query
 * @returns {Array<{name: string}>}
 */
export function filterBranches(branches, query) {
  const q = query.trim().toLowerCase()
  if (!q) return branches
  return branches.filter((b) => b.name.toLowerCase().includes(q))
}

/**
 * git メニュー内のキーボード操作を action に対応付ける pure 関数。
 * @param {string} key KeyboardEvent.key
 * @param {{branchCount: number, selectedIndex: number}} state
 * @returns {{type:'move',index:number}|{type:'checkout'}|{type:'op',op:string}|{type:'close'}|null}
 */
export function menuKeyAction(key, { branchCount, selectedIndex }) {
  switch (key) {
    case 'ArrowDown':
      return { type: 'move', index: Math.min(selectedIndex + 1, Math.max(branchCount - 1, 0)) }
    case 'ArrowUp':
      return { type: 'move', index: Math.max(selectedIndex - 1, 0) }
    case 'Enter':
      return branchCount > 0 ? { type: 'checkout' } : null
    case 'Escape':
      return { type: 'close' }
    case 'p':
    case 'P':
      return { type: 'op', op: 'pull' }
    case 'u':
    case 'U':
      return { type: 'op', op: 'push' }
    case 'f':
    case 'F':
      return { type: 'op', op: 'fetch' }
    default:
      return null
  }
}
