import assert from 'node:assert'
import { paneWsURL, b64ToBytes, isDesktop } from './termTransport.js'

// paneWsURL: ws for http
assert.equal(
  paneWsURL({ protocol: 'http:', host: 'localhost:8080' }, 'abc'),
  'ws://localhost:8080/api/panes/abc/io',
)
// paneWsURL: wss for https
assert.equal(
  paneWsURL({ protocol: 'https:', host: 'example.com' }, 'p1'),
  'wss://example.com/api/panes/p1/io',
)
// b64ToBytes round-trip
const bytes = b64ToBytes(Buffer.from('hello').toString('base64'))
assert.deepEqual(Array.from(bytes), Array.from(new TextEncoder().encode('hello')))
// isDesktop is false without window globals
assert.equal(isDesktop(), false)

console.log('termTransport helpers: OK')
