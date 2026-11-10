/**
 * VEYLIX — UI truth audit (`npm run audit`).
 *
 * The product rule this enforces (docs/new_reqierment/new_update.md §1 / §13):
 * nothing in the UI may state something the backend cannot confirm. The two
 * concrete ways that rule broke in this codebase were
 *
 *  1. a screen claiming an API does not exist after it shipped
 *     ("no API yet", "demo data", "data is mock"), and
 *  2. a screen shipping invented business numbers (hard-coded LKR prices,
 *     fake server rows) — the mock stores were deleted for exactly this reason.
 *
 * Both were found by hand during the 2026-10-24 audit and then fixed; this
 * script is what stops them coming back, and it is cheap enough to run on every
 * commit (no server, no network, no browser).
 *
 * Usage:
 *   node scripts/audit-ui-truth.mjs             fail on findings in src/
 *   node scripts/audit-ui-truth.mjs --json      machine-readable output
 *   node scripts/audit-ui-truth.mjs --self-test prove the rules still fire
 *
 * Exit code 1 on any finding so it can gate CI.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

// fileURLToPath (not `.pathname`) because this repository lives under a path
// with a space in it — `H:\my apps\…` — and `.pathname` keeps it percent-encoded.
const SRC = fileURLToPath(new URL('../src/', import.meta.url))
const ROOT = fileURLToPath(new URL('../', import.meta.url))

/** Files that are allowed to talk about mocks — they document history in a comment. */
const ALLOWED_PATHS = [
  // Stores/scripts carry "this used to be mock" comments; those are the record
  // of a fix, not a claim about the present, and they live in block comments.
  /stores[\\/]/,
  /vite-env\.d\.ts$/,
  /lib[\\/]devMode\.ts$/,
]

/**
 * Phrases that assert a capability does not exist. Any hit is a finding: either
 * the API shipped (fix the copy) or it did not (say what is missing instead of
 * naming an internal stage such as "P3"/"E6" that the user cannot act on).
 */
const FORBIDDEN_COPY = [
  { re: /\bdemo data\b/i, why: 'claims the screen shows demo data' },
  { re: /\bdata is mock\b/i, why: 'claims the data is mock' },
  { re: /\bdemo build\b/i, why: 'calls the product a demo build' },
  { re: /\bno API yet\b/i, why: 'claims an endpoint does not exist' },
  { re: /\b(api|endpoint) is not built yet\b/i, why: 'claims an endpoint does not exist' },
  { re: /\b(in|is|at|from|until|after|before)\s+Stage\s+E\d\b/i, why: 'internal stage id used as user-facing copy' },
]

/** Business figures that must come from the backend, never from a literal. */
const INVENTED_MONEY = [
  {
    re: /\b(wholesale|retail)\s*:\s*\d{3,}\b/i,
    why: 'hard-coded LKR wholesale/retail price',
  },
]

/**
 * A nullish coalesce on a classification field (D-063).
 *
 * `offer_kind ?? tag` looks right and is wrong: the server sends an EMPTY STRING
 * for a VPN with no package, and `"" ?? x` never falls back because an empty
 * string is not nullish. That one character filed an assigned, locked PREMIUM VPN
 * under UNTAGGED on the customer's own screen while "My Premium VPNs" claimed the
 * customer had none. `||` is the operator that falls through, and the server now
 * resolves the kind as well, so this rule keeps the mistake from returning.
 */
const KIND_FALLBACK = [
  {
    re: /\b(offer_kind|offerKind|v\.tag)\s*\?\?\s*(v\.tag|tag|v\.offer_kind|offerKind)\b/,
    why: 'nullish coalesce on a classification field — "" does not fall back; use || (D-063)',
  },
]

/** Nested walk of src/. */
function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) out.push(...walk(full))
    else if (/\.(tsx?|jsx?)$/.test(name)) out.push(full)
  }
  return out
}

/**
 * Removes block and line comments so a historical note ("this used to say
 * 'demo data'") is not flagged. Keeps line numbers stable by replacing the
 * comment text with spaces.
 */
function stripComments(source) {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:])\/\/[^\n]*/g, (m, lead) => lead + ' '.repeat(m.length - lead.length))
}

/** Every rule, in one list so the scanner and the self-test cannot drift. */
const RULES = [...FORBIDDEN_COPY, ...INVENTED_MONEY, ...KIND_FALLBACK]

/** Returns the findings for one source string (used by both scan and self-test). */
function scanSource(rel, source) {
  const hits = []
  stripComments(source)
    .split(/\r?\n/)
    .forEach((line, i) => {
      for (const rule of RULES) {
        if (rule.re.test(line)) {
          hits.push({ file: rel, line: i + 1, why: rule.why, text: line.trim().slice(0, 140) })
        }
      }
    })
  return hits
}

const findings = []

for (const file of walk(SRC)) {
  const rel = relative(ROOT, file).split(sep).join('/')
  if (ALLOWED_PATHS.some((re) => re.test(rel))) continue
  findings.push(...scanSource(rel, readFileSync(file, 'utf8')))
}

// --- self-test -------------------------------------------------------------
// Without this, a typo in a regex silently turns the gate into a no-op that
// always prints OK. Each case is a string that MUST fire and one that MUST NOT.
if (process.argv.includes('--self-test')) {
  const mustFire = [
    ['a.tsx', 'subtitle="Platform health at a glance (demo data)"'],
    ['a.tsx', '© 2026 Veylix — demo build, data is mock (D-006).'],
    ['a.tsx', '<MetricCard delta="no API yet (P5)" />'],
    ['a.tsx', 'the offer catalogue API is not built yet'],
    ['a.tsx', 'the connection link is issued in Stage E6'],
    ['a.tsx', "{ id: 'r1', name: 'Premium 20 GB', wholesale: 600, retail: 900 },"],
    ['a.tsx', 'kind: toKind(v.offer_kind ?? v.tag),'],
    ['a.tsx', '<VpnKindBadge kind={toKind(v.offerKind ?? tag)} />'],
  ]
  const mustNotFire = [
    ['a.tsx', 'UI truth audit: no endpoint is missing here'],
    ['a.tsx', '// this screen used to say "demo data" before 2026-10-24'],
    ['a.tsx', '/* legacy copy: no API yet (P5) */'],
    ['a.tsx', 'const amountCents = payment.amount_cents'],
    // The fix for the rule above must not itself be flagged (D-063).
    ['a.tsx', 'kind: toKind(v.offer_kind || v.tag, assigned),'],
    ['a.tsx', 'offerName: v.offer_name ?? null,'],
  ]

  let failures = 0
  for (const [file, src] of mustFire) {
    if (scanSource(file, src).length === 0) {
      failures++
      console.error(`  SELF-TEST FAIL (should fire): ${src}`)
    }
  }
  for (const [file, src] of mustNotFire) {
    if (scanSource(file, src).length > 0) {
      failures++
      console.error(`  SELF-TEST FAIL (should not fire): ${src}`)
    }
  }
  console.log(
    failures === 0
      ? `UI truth audit self-test: OK — ${mustFire.length} positive + ${mustNotFire.length} negative cases.`
      : `UI truth audit self-test: ${failures} failure(s).`,
  )
  process.exit(failures === 0 ? 0 : 1)
}

const wantJson = process.argv.includes('--json')
if (wantJson) {
  console.log(JSON.stringify({ findings, count: findings.length }, null, 2))
} else if (findings.length === 0) {
  console.log('UI truth audit: OK — no unwired/demo claims and no invented money literals in src/.')
} else {
  console.error(`UI truth audit: ${findings.length} finding(s)`)
  for (const f of findings) {
    console.error(`  ${f.file}:${f.line}  ${f.why}\n      ${f.text}`)
  }
  console.error('\nFix the copy, or wire the screen to the real endpoint — see docs/API_CONTRACT.md.')
}

process.exit(findings.length === 0 ? 0 : 1)
