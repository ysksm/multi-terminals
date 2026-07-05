/**
 * Pure helpers for keyboard-driven workspace navigation.
 * Workspaces are identified by objects with an `id` field, in sidebar order.
 */

/**
 * Returns the id of the workspace `delta` steps away from the current one,
 * wrapping around at both ends. When the current id is missing or not in the
 * list, returns the first workspace so keyboard users can enter the list.
 *
 * @param {{id: string}[]} workspaces - Workspaces in sidebar order
 * @param {string|null} currentId - Currently selected workspace id
 * @param {number} delta - Steps to move (-1: prev, +1: next)
 * @returns {string|null} Target workspace id, or null if the list is empty
 */
export function cycleWorkspaceId(workspaces, currentId, delta) {
  if (!workspaces?.length) return null
  const idx = workspaces.findIndex((w) => w.id === currentId)
  if (idx < 0) return workspaces[0].id
  const n = workspaces.length
  return workspaces[(((idx + delta) % n) + n) % n].id
}

/**
 * Returns the id of the workspace at the given 0-based index,
 * or null if the index is out of range.
 *
 * @param {{id: string}[]} workspaces - Workspaces in sidebar order
 * @param {number} index - 0-based index (Cmd+1 => 0)
 * @returns {string|null}
 */
export function workspaceIdAt(workspaces, index) {
  if (!workspaces?.length) return null
  if (index < 0 || index >= workspaces.length) return null
  return workspaces[index].id
}
