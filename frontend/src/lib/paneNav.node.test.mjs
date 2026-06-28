import assert from 'node:assert'
import { neighborSlot } from './paneNav.js'

// grid_2x2 (cols=2, rows=2)
// slot 0 (row=0, col=0)
assert.equal(neighborSlot(0, 2, 2, 'right'), 1,   'grid_2x2: slot0 right=1')
assert.equal(neighborSlot(0, 2, 2, 'down'),  2,   'grid_2x2: slot0 down=2')
assert.equal(neighborSlot(0, 2, 2, 'left'),  null, 'grid_2x2: slot0 left=null')
assert.equal(neighborSlot(0, 2, 2, 'up'),    null, 'grid_2x2: slot0 up=null')
// slot 3 (row=1, col=1)
assert.equal(neighborSlot(3, 2, 2, 'left'),  2,    'grid_2x2: slot3 left=2')
assert.equal(neighborSlot(3, 2, 2, 'up'),    1,    'grid_2x2: slot3 up=1')
assert.equal(neighborSlot(3, 2, 2, 'right'), null, 'grid_2x2: slot3 right=null')
assert.equal(neighborSlot(3, 2, 2, 'down'),  null, 'grid_2x2: slot3 down=null')

// split_vertical (cols=2, rows=1)
assert.equal(neighborSlot(0, 2, 1, 'right'), 1,    'split_vertical: slot0 right=1')
assert.equal(neighborSlot(1, 2, 1, 'left'),  0,    'split_vertical: slot1 left=0')
assert.equal(neighborSlot(0, 2, 1, 'up'),    null, 'split_vertical: slot0 up=null')
assert.equal(neighborSlot(0, 2, 1, 'down'),  null, 'split_vertical: slot0 down=null')
assert.equal(neighborSlot(1, 2, 1, 'up'),    null, 'split_vertical: slot1 up=null')
assert.equal(neighborSlot(1, 2, 1, 'down'),  null, 'split_vertical: slot1 down=null')

// split_horizontal (cols=1, rows=2)
assert.equal(neighborSlot(0, 1, 2, 'down'),  1,    'split_horizontal: slot0 down=1')
assert.equal(neighborSlot(1, 1, 2, 'up'),    0,    'split_horizontal: slot1 up=0')
assert.equal(neighborSlot(0, 1, 2, 'left'),  null, 'split_horizontal: slot0 left=null')
assert.equal(neighborSlot(0, 1, 2, 'right'), null, 'split_horizontal: slot0 right=null')
assert.equal(neighborSlot(1, 1, 2, 'left'),  null, 'split_horizontal: slot1 left=null')
assert.equal(neighborSlot(1, 1, 2, 'right'), null, 'split_horizontal: slot1 right=null')

// single (cols=1, rows=1)
assert.equal(neighborSlot(0, 1, 1, 'left'),  null, 'single: slot0 left=null')
assert.equal(neighborSlot(0, 1, 1, 'right'), null, 'single: slot0 right=null')
assert.equal(neighborSlot(0, 1, 1, 'up'),    null, 'single: slot0 up=null')
assert.equal(neighborSlot(0, 1, 1, 'down'),  null, 'single: slot0 down=null')

console.log('paneNav: OK')
