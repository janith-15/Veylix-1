/**
 * Self-test for the browser-side ping (P-05).
 *
 * `measureClientPing` needs a real socket, but `parseTarget` is the part that
 * can silently be wrong — and a wrong target means a latency measured against
 * the wrong host, which looks perfectly healthy. This pins the parsing rules.
 *
 * Run: `npm run ping:selftest`
 */
import { parseTarget } from '../src/lib/clientPing.ts'

let passed = 0
const failures = []

function eq(label, actual, expected) {
  const a = JSON.stringify(actual)
  const e = JSON.stringify(expected)
  if (a === e) {
    passed++
  } else {
    failures.push(`${label}: got ${a}, want ${e}`)
  }
}

// --- IPv4 host:port ---------------------------------------------------------
eq('plain v4 host:port', parseTarget('vpn.example.com:443'), { host: 'vpn.example.com', port: 443 })
eq('bare ip', parseTarget('203.0.113.7:2087'), { host: '203.0.113.7', port: 2087 })
eq('high port', parseTarget('example.com:65535'), { host: 'example.com', port: 65535 })

// --- IPv6 -------------------------------------------------------------------
eq('bracketed v6', parseTarget('[2001:db8::1]:443'), { host: '2001:db8::1', port: 443 })
eq('bracketed v6 high port', parseTarget('[::1]:8443'), { host: '::1', port: 8443 })

// --- Refusals: anything ambiguous must be null, never a guess ---------------
eq('empty', parseTarget(''), null)
eq('whitespace', parseTarget('   '), null)
eq('no port', parseTarget('example.com'), null)
eq('trailing colon', parseTarget('example.com:'), null)
eq('non-numeric port', parseTarget('example.com:https'), null)
eq('port zero', parseTarget('example.com:0'), null)
eq('port too high', parseTarget('example.com:65536'), null)
eq('empty host', parseTarget(':443'), null)
eq('unbracketed v6 is ambiguous', parseTarget('2001:db8::1:443'), { host: '2001:db8::1', port: 443 })

// --- Trimming ---------------------------------------------------------------
eq('surrounding spaces are trimmed', parseTarget('  vpn.example.com:443  '), {
  host: 'vpn.example.com',
  port: 443,
})

if (failures.length > 0) {
  console.error(`client-ping self-test: FAILED — ${failures.length} case(s)`)
  for (const f of failures) console.error('  - ' + f)
  process.exit(1)
}

console.log(`client-ping self-test: OK — ${passed} cases.`)
