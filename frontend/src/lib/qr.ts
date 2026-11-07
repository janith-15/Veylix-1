/**
 * A QR encoder, in-repo and dependency-free (D-033.2).
 *
 * Why not a library: a connection URI contains the customer's credential. Sending
 * it to a third-party QR service would leak it, and adding an npm package for a
 * small algorithm is a supply-chain risk on a 500 MB host.
 *
 * Scope is deliberately the product's need: byte mode, error-correction level M,
 * versions 1–15. A `vless://` URI fits from version 9; a `vmess://` link is a
 * base64 JSON payload and routinely 250–360 bytes, which version 10 could not
 * hold — it refused the QR instead of shipping a truncated credential (M7-14,
 * found by `npm run qr:selftest`). Version 15 covers 412 bytes at level M, which
 * is every link a 3X-UI panel emits for the supported protocols. Anything longer
 * is still refused with an error the UI can show, never truncated.
 */

/** Total codewords per version (data + error correction). */
const TOTAL_CODEWORDS = [
  0, 26, 44, 70, 100, 134, 172, 196, 242, 292, 346, 404, 466, 532, 581, 655,
  733, 815, 901, 991, 1085,
]

/**
 * Reed–Solomon block layout for level M, by version, as
 * `[blockCount, dataCodewordsPerBlock, eccCodewordsPerBlock]` groups.
 *
 * The numbers come from the spec's level-M table. They are written out rather
 * than derived because a wrong derivation produces a QR that *looks* fine and
 * never scans — the worst possible failure for a credential.
 */
const RS_BLOCKS_M: Record<number, Array<[number, number, number]>> = {
  1: [[1, 16, 10]],
  2: [[1, 28, 16]],
  3: [[1, 44, 26]],
  4: [[2, 32, 18]],
  5: [[2, 43, 24]],
  6: [[4, 27, 16]],
  7: [[4, 31, 18]],
  8: [[2, 38, 22], [2, 39, 22]],
  9: [[3, 36, 22], [2, 37, 22]],
  10: [[4, 43, 26], [1, 44, 26]],
  // 11–15 added for M7-14. Each row's data+ecc total equals TOTAL_CODEWORDS[version]
  // above — the encoder asserts that at run time, so a bad row cannot draw a code
  // that looks fine and never scans.
  11: [[1, 50, 30], [4, 51, 30]],
  12: [[6, 36, 22], [2, 37, 22]],
  13: [[8, 37, 22], [1, 38, 22]],
  14: [[4, 40, 24], [5, 41, 24]],
  15: [[5, 41, 24], [5, 42, 24]],
  // 16–20 added for a REAL panel link that version 15 refused (M7-21): a
  // 442-byte `vless://` URI carrying alpn/fp/sni/sid parameters. The encoder said
  // "too long to encode" while the customer had no QR at all — the link still had
  // text and a Copy button, so the gap was easy to miss without a live panel.
  16: [[7, 45, 28], [3, 46, 28]],
  17: [[10, 46, 28], [1, 47, 28]],
  18: [[9, 43, 26], [4, 44, 26]],
  19: [[3, 44, 26], [11, 45, 26]],
  20: [[3, 41, 26], [13, 42, 26]],
}

/** The largest version this encoder draws. */
const MAX_VERSION = 20

/**
 * Alignment-pattern centre coordinates per version.
 *
 * The gaps between the three last centres alternate by −2 for every second
 * version, so 11–15 read [6, 30, 54] [6, 32, 58], [6, 34, 62], [6, 26, 46, 66],
 * [6, 26, 48, 70] — the spec's table for level M.
 *
 * 16–20 keep the same alternation: [6, 26, 50, 74], [6, 30, 54, 78],
 * [6, 30, 56, 82], [6, 30, 58, 86], [6, 34, 62, 90]. The alignment count is what
 * changes — a wrong centre is a code that still looks like a QR and only fails
 * when a phone tries to read it.
 */
const ALIGNMENT = [
  [], [], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34],
  [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50],
  [6, 30, 54], [6, 32, 58], [6, 34, 62], [6, 26, 46, 66], [6, 26, 48, 70],
  [6, 26, 50, 74], [6, 30, 54, 78], [6, 30, 56, 82], [6, 30, 58, 86], [6, 34, 62, 90],
]

/** The 15-bit format information for level M (mask 0–7), already XORed. */
const FORMAT_M = [0x5412, 0x5125, 0x5e7c, 0x5b4b, 0x45f9, 0x40ce, 0x4f97, 0x4aa0]

export interface QrMatrix {
  /** module count per side */
  size: number
  /** modules[y][x] — true is dark */
  modules: boolean[][]
  version: number
}

/** Flattens the level-M block table into one entry per block. */
function blocks(version: number): Array<{ data: number; ecc: number }> {
  const out: Array<{ data: number; ecc: number }> = []
  for (const [count, data, ecc] of RS_BLOCKS_M[version]) {
    for (let i = 0; i < count; i++) out.push({ data, ecc })
  }
  return out
}

/** A growing bit buffer, MSB first. */
class BitBuffer {
  readonly bits: number[] = []
  append(value: number, length: number): void {
    for (let i = length - 1; i >= 0; i--) this.bits.push((value >>> i) & 1)
  }
}

/** UTF-8 bytes for `text` (QR byte mode is byte-oriented, not code-point). */
function utf8(text: string): number[] {
  const out: number[] = []
  for (const ch of text) {
    const cp = ch.codePointAt(0) ?? 0
    if (cp < 0x80) out.push(cp)
    else if (cp < 0x800) out.push(0xc0 | (cp >> 6), 0x80 | (cp & 0x3f))
    else if (cp < 0x10000) {
      out.push(0xe0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f))
    } else {
      out.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f),
        0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f))
    }
  }
  return out
}

/** GF(2^8) multiplication modulo x^8 + x^4 + x^3 + x^2 + 1. */
function gfMul(x: number, y: number): number {
  let z = 0
  for (let i = 7; i >= 0; i--) {
    z = (z << 1) ^ ((z >>> 7) * 0x11d)
    z ^= ((y >>> i) & 1) * x
  }
  return z & 0xff
}

/** The Reed–Solomon divisor polynomial for `degree` ECC codewords. */
function rsDivisor(degree: number): number[] {
  const result = new Array<number>(degree).fill(0)
  result[degree - 1] = 1
  let root = 1
  for (let i = 0; i < degree; i++) {
    for (let j = 0; j < result.length; j++) {
      result[j] = gfMul(result[j], root)
      if (j + 1 < result.length) result[j] ^= result[j + 1]
    }
    root = gfMul(root, 0x02)
  }
  return result
}

/** The ECC codewords for one data block. */
function rsRemainder(data: number[], divisor: number[]): number[] {
  const result = divisor.map(() => 0)
  for (const b of data) {
    const factor = b ^ (result.shift() ?? 0)
    result.push(0)
    for (let i = 0; i < divisor.length; i++) result[i] ^= gfMul(divisor[i], factor)
  }
  return result
}


/**
 * Encodes `text` and returns the module matrix.
 *
 * Throws when the text does not fit: a truncated QR is a QR that scans to the
 * wrong string, which for a credential is worse than an error message.
 */
export function encodeQr(text: string): QrMatrix {
  const data = utf8(text)

  // The smallest version at level M whose data capacity holds this text.
  let version = 0
  for (let v = 1; v <= MAX_VERSION; v++) {
    const capacity = blocks(v).reduce((sum, b) => sum + b.data, 0) * 8
    const needed = 4 + (v < 10 ? 8 : 16) + data.length * 8
    if (needed <= capacity) {
      version = v
      break
    }
  }
  if (version === 0) {
    const max = blocks(MAX_VERSION).reduce((sum, b) => sum + b.data, 0)
    throw new Error(
      `QR: this link is too long to encode (${data.length} bytes; the limit is ${max - 3} at version ${MAX_VERSION})`,
    )
  }

  const layout = blocks(version)
  const capacityBits = layout.reduce((sum, b) => sum + b.data, 0) * 8

  // ---- data bit stream, byte mode ---------------------------------------
  const bb = new BitBuffer()
  bb.append(0x4, 4)
  bb.append(data.length, version < 10 ? 8 : 16)
  for (const b of data) bb.append(b, 8)
  bb.append(0, Math.min(4, capacityBits - bb.bits.length))
  while (bb.bits.length % 8 !== 0) bb.bits.push(0)
  for (let pad = 0xec; bb.bits.length < capacityBits; pad ^= 0xec ^ 0x11) bb.append(pad, 8)

  const dataCodewords: number[] = []
  for (let i = 0; i < bb.bits.length; i += 8) {
    let byte = 0
    for (let j = 0; j < 8; j++) byte = (byte << 1) | bb.bits[i + j]
    dataCodewords.push(byte)
  }

  // ---- split, add ECC, interleave ---------------------------------------
  const dataBlocks: number[][] = []
  const eccBlocks: number[][] = []
  let offset = 0
  for (const b of layout) {
    const chunk = dataCodewords.slice(offset, offset + b.data)
    offset += b.data
    dataBlocks.push(chunk)
    eccBlocks.push(rsRemainder(chunk, rsDivisor(b.ecc)))
  }
  const codewords: number[] = []
  const maxData = Math.max(...layout.map((b) => b.data))
  const maxEcc = Math.max(...layout.map((b) => b.ecc))
  for (let i = 0; i < maxData; i++) for (const c of dataBlocks) if (i < c.length) codewords.push(c[i])
  for (let i = 0; i < maxEcc; i++) for (const c of eccBlocks) if (i < c.length) codewords.push(c[i])
  if (codewords.length !== TOTAL_CODEWORDS[version]) {
    throw new Error(`QR: codeword count ${codewords.length} does not match version ${version}`)
  }

  return drawMatrix(version, codewords)
}


/** Draws the function patterns, then places the data under the best mask. */
function drawMatrix(version: number, codewords: number[]): QrMatrix {
  const size = version * 4 + 17
  const modules: boolean[][] = Array.from({ length: size }, () => new Array<boolean>(size).fill(false))
  const fixed: boolean[][] = Array.from({ length: size }, () => new Array<boolean>(size).fill(false))

  const set = (x: number, y: number, dark: boolean): void => {
    modules[y][x] = dark
    fixed[y][x] = true
  }

  // Timing patterns.
  for (let i = 0; i < size; i++) {
    set(6, i, i % 2 === 0)
    set(i, 6, i % 2 === 0)
  }
  // Finder patterns with their separators.
  for (const [cx, cy] of [[3, 3], [size - 4, 3], [3, size - 4]] as const) {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const x = cx + dx
        const y = cy + dy
        if (x < 0 || y < 0 || x >= size || y >= size) continue
        const dist = Math.max(Math.abs(dx), Math.abs(dy))
        set(x, y, dist !== 2 && dist !== 4)
      }
    }
  }
  // Alignment patterns, skipping the three finder corners.
  const centres = ALIGNMENT[version]
  for (const cy of centres) {
    for (const cx of centres) {
      if ((cx === 6 && cy === 6) || (cx === 6 && cy === size - 7) || (cx === size - 7 && cy === 6)) continue
      for (let dy = -2; dy <= 2; dy++) {
        for (let dx = -2; dx <= 2; dx++) {
          set(cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1)
        }
      }
    }
  }
  // Reserve the format areas (written after the mask is known) and the dark module.
  for (let i = 0; i < 9; i++) {
    if (!fixed[8][i]) set(i, 8, false)
    if (!fixed[i][8]) set(8, i, false)
  }
  for (let i = 0; i < 8; i++) {
    if (!fixed[8][size - 1 - i]) set(size - 1 - i, 8, false)
    if (!fixed[size - 1 - i][8]) set(8, size - 1 - i, false)
  }
  set(8, size - 8, true)

  const place = (mask: number): void => {
    let bit = 0
    for (let right = size - 1; right >= 1; right -= 2) {
      if (right === 6) right = 5 // skip the vertical timing column
      for (let vert = 0; vert < size; vert++) {
        for (let j = 0; j < 2; j++) {
          const x = right - j
          const upward = ((right + 1) & 2) === 0
          const y = upward ? size - 1 - vert : vert
          if (fixed[y][x]) continue
          let dark = false
          if (bit < codewords.length * 8) {
            dark = ((codewords[bit >>> 3] >>> (7 - (bit & 7))) & 1) === 1
            bit++
          }
          modules[y][x] = dark !== masked(mask, x, y)
        }
      }
    }
  }

  let bestMask = 0
  let bestScore = Infinity
  for (let mask = 0; mask < 8; mask++) {
    place(mask)
    writeFormat(modules, size, mask)
    if (version >= 7) writeVersion(modules, size, version)
    const score = penalty(modules, size)
    if (score < bestScore) {
      bestScore = score
      bestMask = mask
    }
  }
  // Redraw the winner: the loop above leaves the last mask in place.
  place(bestMask)
  writeFormat(modules, size, bestMask)
  if (version >= 7) writeVersion(modules, size, version)

  return { size, modules, version }
}

/** The eight mask predicates from the spec. */
function masked(mask: number, x: number, y: number): boolean {
  switch (mask) {
    case 0: return (x + y) % 2 === 0
    case 1: return y % 2 === 0
    case 2: return x % 3 === 0
    case 3: return (x + y) % 3 === 0
    case 4: return (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0
    case 5: return ((x * y) % 2) + ((x * y) % 3) === 0
    case 6: return (((x * y) % 2) + ((x * y) % 3)) % 2 === 0
    default: return (((x + y) % 2) + ((x * y) % 3)) % 2 === 0
  }
}

/** Writes the 15-bit format information for level M and `mask`. */
function writeFormat(modules: boolean[][], size: number, mask: number): void {
  const bits = FORMAT_M[mask]
  const bit = (i: number): boolean => ((bits >>> i) & 1) === 1
  for (let i = 0; i <= 5; i++) modules[8][i] = bit(i)
  modules[8][7] = bit(6)
  modules[8][8] = bit(7)
  modules[7][8] = bit(8)
  for (let i = 9; i < 15; i++) modules[14 - i][8] = bit(i)
  for (let i = 0; i < 8; i++) modules[size - 1 - i][8] = bit(i)
  for (let i = 8; i < 15; i++) modules[8][size - 15 + i] = bit(i)
  modules[size - 8][8] = true
}

/** Writes the 18-bit version information (version 7 and above). */
function writeVersion(modules: boolean[][], size: number, version: number): void {
  let rem = version
  for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25)
  const bits = (version << 12) | rem
  for (let i = 0; i < 18; i++) {
    const dark = ((bits >>> i) & 1) === 1
    const a = size - 11 + (i % 3)
    const b = Math.floor(i / 3)
    modules[b][a] = dark
    modules[a][b] = dark
  }
}


/**
 * The four penalty rules, used to pick the mask that scans most reliably.
 *
 * These do not affect correctness — every mask produces a valid QR — but they
 * decide how well a phone camera copes, so the scoring follows the spec exactly.
 */
function penalty(modules: boolean[][], size: number): number {
  let score = 0

  // Rule 1 — runs of five or more same-coloured modules.
  for (let y = 0; y < size; y++) {
    let run = 1
    for (let x = 1; x < size; x++) {
      if (modules[y][x] === modules[y][x - 1]) run++
      else {
        if (run >= 5) score += 3 + (run - 5)
        run = 1
      }
    }
    if (run >= 5) score += 3 + (run - 5)
  }
  for (let x = 0; x < size; x++) {
    let run = 1
    for (let y = 1; y < size; y++) {
      if (modules[y][x] === modules[y - 1][x]) run++
      else {
        if (run >= 5) score += 3 + (run - 5)
        run = 1
      }
    }
    if (run >= 5) score += 3 + (run - 5)
  }

  // Rule 2 — 2×2 blocks of one colour.
  for (let y = 0; y < size - 1; y++) {
    for (let x = 0; x < size - 1; x++) {
      const c = modules[y][x]
      if (c === modules[y][x + 1] && c === modules[y + 1][x] && c === modules[y + 1][x + 1]) score += 3
    }
  }

  // Rule 3 — finder-like 1:1:3:1:1 patterns.
  const pattern = [true, false, true, true, true, false, true]
  const matches = (get: (i: number) => boolean, at: number): boolean => {
    for (let i = 0; i < 7; i++) if (get(at + i) !== pattern[i]) return false
    return true
  }
  for (let y = 0; y < size; y++) {
    for (let x = 0; x + 7 <= size; x++) {
      if (matches((i) => modules[y][i], x)) score += 40
      if (matches((i) => modules[i][y], x)) score += 40
    }
  }

  // Rule 4 — distance from a 50/50 dark/light balance.
  let dark = 0
  for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) if (modules[y][x]) dark++
  score += Math.floor(Math.abs((dark * 100) / (size * size) - 50) / 5) * 10
  return score
}

