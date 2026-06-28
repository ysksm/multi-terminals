/**
 * Returns the slot index of the neighbor in the given direction,
 * or null if the neighbor would be out of the grid bounds.
 *
 * @param {number} slot - Current slot index (0-based)
 * @param {number} cols - Number of columns in the grid
 * @param {number} rows - Number of rows in the grid
 * @param {'left'|'right'|'up'|'down'} direction - Direction to move
 * @returns {number|null}
 */
export function neighborSlot(slot, cols, rows, direction) {
  const row = Math.floor(slot / cols)
  const col = slot % cols

  let newRow = row
  let newCol = col

  if (direction === 'left') {
    newCol = col - 1
  } else if (direction === 'right') {
    newCol = col + 1
  } else if (direction === 'up') {
    newRow = row - 1
  } else if (direction === 'down') {
    newRow = row + 1
  }

  if (newRow < 0 || newRow >= rows || newCol < 0 || newCol >= cols) {
    return null
  }

  return newRow * cols + newCol
}
