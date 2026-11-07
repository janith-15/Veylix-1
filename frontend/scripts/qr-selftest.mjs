import assert from 'node:assert/strict'
import { encodeQr } from '../src/lib/qr.ts'

/**
 * A structural self-test for the in-repo QR encoder (D-033).
 *
 * It cannot prove that a phone camera reads the code, but it proves the two
 * things that make that impossible when they go wrong:
 *
 *  1. the matrix is structurally a QR — correct size, three finder patterns,
 *     timing patterns, and a quiet zone added by the component;
 *  2. the version grows with the payload, so a longer `vless://` URI does not
 *     silently produce the same fixed-size code; and
 *  3. the emitted version's block table agrees with the spec's codeword count —
 *     the one mistake that yields a code which looks right and never scans.
 *
 * Run with `npm run qr:selftest`.
 */

const pass = []
const fail = []
function check(name, fn) {
  try {
    fn()
    pass.push(name)
  } catch (err) {
    fail.push(`${name}: ${err instanceof Error ? err.message : String(err)}`)
  }
}

const URI = 'vless://5f6a1c4e-1111-2222-3333-444455556666@panel.example:443?type=tcp&security=reality#nimal-paid-1'

check('encodes a real connection URI', () => {
  const qr = encodeQr(URI)
  assert.ok(qr.size >= 21 && qr.size % 4 === 1, `size ${qr.size} is not a valid QR size`)
  assert.equal(qr.version, (qr.size - 17) / 4)
})

check('draws three finder patterns', () => {
  const { modules, size } = encodeQr(URI)
  // The finder is 7×7: dark border, light ring, 3×3 dark core.
  const finderAt = (x0, y0) =>
    modules[y0][x0] &&
    modules[y0 + 6][x0] && modules[y0][x0 + 6] &&
    !modules[y0 + 1][x0 + 1] && !modules[y0 + 5][x0 + 5] &&
    modules[y0 + 2][x0 + 2] && modules[y0 + 4][x0 + 4]
  assert.ok(finderAt(0, 0), 'top-left finder missing')
  assert.ok(finderAt(size - 7, 0), 'top-right finder missing')
  assert.ok(finderAt(0, size - 7), 'bottom-left finder missing')
})

check('draws the timing patterns', () => {
  const { modules } = encodeQr(URI)
  for (let i = 8; i < 12; i++) {
    assert.equal(modules[6][i], i % 2 === 0, `row-6 timing wrong at x=${i}`)
    assert.equal(modules[i][6], i % 2 === 0, `column-6 timing wrong at y=${i}`)
  }
})

check('grows with the payload', () => {
  const short = encodeQr('vless://a@b:1')
  const long = encodeQr(URI + '&padding=' + 'x'.repeat(40))
  assert.ok(long.version > short.version, `version did not grow (${short.version} → ${long.version})`)
})

check('accepts a realistic long URI (about 200 bytes)', () => {
  const uri = URI + '&fp=chrome&pbk=' + 'A'.repeat(43) + '&sid=9f2c&spx=%2F&sni=panel.example'
  assert.ok(uri.length > 150, `the fixture is only ${uri.length} bytes`)
  const qr = encodeQr(uri)
  // Version 20 is the largest this encoder draws; exceeding it is an error, not
  // a truncated code.
  assert.ok(qr.version <= 20)
})

check('refuses text it cannot fit, instead of truncating', () => {
  // Past version 20's cap it must be refused rather than silently shortened.
  assert.throws(() => encodeQr('x'.repeat(700)), /too long/)
})

check('is deterministic', () => {
  const a = encodeQr(URI)
  const b = encodeQr(URI)
  assert.deepEqual(a.modules, b.modules)
})

/**
 * P-10 (`MILESTONE_7_PLAN.md` §6) — the QR must carry the *panel's own* link,
 * not a rebuilt or truncated one (D-033.2/.4).
 *
 * A real `GET /api/vpns/{id}/links` answer is a list of URIs the panel itself
 * returned. The screen encodes the first one, so this fixture is shaped like
 * that answer and checks the two failure modes that would make a scanner connect
 * to the wrong server or fail outright:
 *
 *  1. every link must fit version 15 (the encoder's documented cap) — a refused
 *     link means the customer gets no QR at all;
 *  2. `encodeQr` throws when its block table disagrees with the spec, so a code
 *     that renders is one whose codeword count is exact.
 *
 * The declared byte count cannot be read back without decoding the zig-zag
 * placement, so truncation is proved where it actually lives — in the version
 * selection below, which walks every payload length from 1 to the cap.
 */
check('encodes a full panel link without truncation (vless / vmess / trojan)', () => {
  // Shaped like the panel's answer: URI + the fragment naming the client (D-034).
  const links = {
    vless:
      'vless://5f6a1c4e-1111-2222-3333-444455556666@panel.example:443' +
      '?type=tcp&security=reality&fp=chrome&pbk=' + 'A'.repeat(43) +
      '&sid=9f2c&spx=%2F&sni=panel.example&flow=xtls-rprx-vision' +
      '#nimal%40gmail.com_paid_1',
    vmess:
      'vmess://eyJ2IjoiMiIsInBzIjoibmltYWxAZ21haWwuY29tX3BhaWRfMSIsImFkZCI6InBhbmVsLmV4YW1wbGUiLCJwb3J0IjoiNDQzIiwiaWQiOiI1ZjZhMWM0ZS0xMTExLTIyMjItMzMzMy00NDQ0NTU1NTY2NjYiLCJhaWQiOiIwIiwibmV0IjoidGNwIiwidHlwZSI6Im5vbmUiLCJob3N0IjoiIiwicGF0aCI6Ii8iLCJ0bHMiOiJyZWFsaXR5In0=',
    trojan:
      'trojan://a-very-long-password-12345@panel.example:443' +
      '?type=ws&security=tls&path=%2Fws%3Fed%3D2048&host=panel.example' +
      '&sni=panel.example&alpn=h2%2Chttp%2F1.1&fp=chrome&allowInsecure=0' +
      '#nimal%40gmail.com_paid_2',
  }

  for (const [proto, link] of Object.entries(links)) {
    const bytes = new TextEncoder().encode(link).length
    assert.ok(bytes > 150, `${proto}: the fixture is only ${bytes} bytes — not a realistic link`)
    // The whole point of M7-14: a vmess link is JSON in base64 and overran the
    // old version-10 cap (213 bytes), so the customer saw no QR at all.
    const qr = encodeQr(link)
    assert.ok(
      qr.version >= 1 && qr.version <= 20,
      `${proto}: version ${qr.version} exceeds the encoder's range for a ${bytes}-byte link`,
    )
    assert.equal(qr.version, (qr.size - 17) / 4)
  }

  // The regression itself: this exact class of link must be encodable today.
  const vmessBytes = new TextEncoder().encode(links.vmess).length
  assert.ok(vmessBytes > 213, `the vmess fixture (${vmessBytes} B) must exceed the old v10 cap`)
  assert.ok(encodeQr(links.vmess).version > 10, 'a >213-byte link must use version 11 or above')
})

/**
 * M7-21 — a REAL panel link that version 15 refused.
 *
 * Found by `scripts/verify-vpn-detail.mjs` against a live 3X-UI panel: the
 * customer's `vless://` URI was **442 bytes** (alpn + fp + sni + sid + the client
 * name in the fragment), so the page showed "this link is too long to encode"
 * where a QR belonged. The link text and Copy button still worked, which is why
 * no earlier test caught it — only a real panel emits a link this long.
 *
 * Versions 16–20 now cover 643 bytes. The tables for them come from the spec's
 * level-M RS block table, and the encoder's own codeword assertion is what proves
 * them: a row that is off by one throws instead of drawing an unscannable code.
 */
check('encodes a 442-byte real panel link that version 15 refused', () => {
  // The exact shape of the link the live panel returned for VPN 17: a reality
  // endpoint whose query carries alpn/fp/pbk/sid/sni/spx plus the client's name
  // in the fragment. The panel pads this out with the full parameter set, and the
  // result was 442 bytes — past version 15's 412, so the customer saw an error
  // where their QR belonged.
  const base =
    'vless://a24bda76-0295-4fb9-a23d-292f31d06f7c@difanx4f3.difanx1.ggff.net:443' +
    '?alpn=h2%2Chttp%2F1.1&encryption=none&fp=chrome' +
    '&pbk=S9mQ2pXk7vL4nR8tY1wZ3bC6dF0gH5jK2mN4pQ7sT9uAbCdEfGh' +
    '&security=reality&sid=6f2a1c7b9d3e4f5a&sni=difanx4f3.difanx1.ggff.net' +
    '&spx=%2F&type=tcp&flow=xtls-rprx-vision&headerType=none' +
    '&path=%2F&host=difanx4f3.difanx1.ggff.net&serviceName=&mode=&extra=' +
    '#nimal%40gmail.com_paid_1-2026-10-25'
  // Pad to exactly 442 bytes with a realistic client-name suffix, so the case is
  // the measured length rather than "roughly long".
  const real = base + 'x'.repeat(442 - new TextEncoder().encode(base).length)
  const bytes = new TextEncoder().encode(real).length
  assert.equal(bytes, 442, `the fixture must be 442 bytes, got ${bytes}`)
  assert.ok(bytes > 412, `the fixture is ${bytes} bytes — it must exceed the old 412 cap`)

  const qr = encodeQr(real)
  assert.ok(qr.version > 15, `a ${bytes}-byte link must use version 16 or above, got ${qr.version}`)
  assert.equal(qr.version, (qr.size - 17) / 4)
})

/**
 * The table rows must agree with the spec exactly: data + ecc codewords per
 * version equals the spec's total. A row that is off by one draws a code that
 * looks plausible and never scans, so the whole range is exercised — not only the
 * versions the fixtures happen to reach. The encoder throws when a version's
 * block table does not add up, so reaching the top version at all is the proof
 * that every row is consistent.
 */
check('version 20 is reachable and the 666-byte cap resolves exactly', () => {
  const seen = new Set()
  for (let n = 1; n <= 666; n++) {
    const qr = encodeQr('x'.repeat(n))
    seen.add(qr.version)
    if (qr.version < 1 || qr.version > 20) {
      throw new Error(`payload of ${n} bytes produced version ${qr.version}`)
    }
  }
  const missing = []
  for (let v = 1; v <= 20; v++) if (!seen.has(v)) missing.push(v)
  // Every version 1–20 must be selected somewhere in the walk: a gap would mean a
  // table row is wrong and the encoder skipped a version.
  if (missing.length > 0) throw new Error(`versions never selected: ${missing.join(', ')}`)

  // One byte past the cap is refused, not squeezed in or truncated.
  assert.throws(() => encodeQr('x'.repeat(667)), /too long/)
  // And the size formula still holds at the top of the range.
  const top = encodeQr('x'.repeat(666))
  assert.equal(top.version, 20)
  assert.equal(top.size, 20 * 4 + 17)
})

if (fail.length > 0) {
  for (const f of fail) console.error(`  QR SELF-TEST FAIL — ${f}`)
  console.error(`QR encoder self-test: ${fail.length} failure(s).`)
  process.exit(1)
}
console.log(`QR encoder self-test: OK — ${pass.length} cases.`)
