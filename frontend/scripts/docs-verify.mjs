/**
 * VEYLIX — documentation drift check (`npm run docs:verify`).
 *
 * The defect this prevents: `README.md` claimed "11 migrations · schema v11" and
 * "84 HTTP routes" for days while the tree had 16 and 115 (IMPROVEMENT_LEDGER
 * F-056). Nothing recomputed those numbers, so they silently became false — and a
 * new reader trusts the README over the code.
 *
 * This script derives the counts from the tree and fails when a document disagrees.
 * It reads, never writes: a human decides the new wording.
 *
 * Usage:
 *   node scripts/docs-verify.mjs            compare and report
 *   node scripts/docs-verify.mjs --json     machine-readable output
 *
 * Exit code 1 on any drift so it can gate CI.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

// fileURLToPath (not `.pathname`) because this repository lives under a path with
// a space in it — `H:\my apps\…` — and `.pathname` keeps it percent-encoded.
const ROOT = fileURLToPath(new URL('../../', import.meta.url))

const BACKEND = join(ROOT, 'backend')
const MIGRATIONS = join(BACKEND, 'internal', 'store', 'migrations')
const README = join(ROOT, 'README.md')

/** Every .go file under dir, skipping _test.go when asked. */
function walk(dir, { skipTests = false } = {}) {
  const out = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) out.push(...walk(full, { skipTests }))
    else if (entry.name.endsWith('.go') && !(skipTests && entry.name.endsWith('_test.go'))) out.push(full)
  }
  return out
}

/**
 * Counts a documented fact must agree with.
 *
 * Each is derived from the tree, never typed by hand, so the check cannot drift
 * with the thing it is checking.
 */
export function actualFacts() {
  const migrations = readdirSync(MIGRATIONS).filter((f) => f.endsWith('.sql')).length

  // A route is a `mux.Handle("<METHOD> /path"` or `mux.HandleFunc("...")` in
  // non-test code. Method-prefixed patterns only: a bare "/" (the SPA fallback)
  // is not an API route.
  const routePattern = /Handle(?:Func)?\("(GET|POST|PUT|PATCH|DELETE) /
  let routes = 0
  for (const file of walk(join(BACKEND, 'internal'), { skipTests: true })) {
    const src = readFileSync(file, 'utf8')
    for (const line of src.split('\n')) if (routePattern.test(line)) routes++
  }

  const testFuncs = (() => {
    let n = 0
    for (const file of walk(join(BACKEND, 'internal'))) {
      if (!file.endsWith('_test.go')) continue
      for (const line of readFileSync(file, 'utf8').split('\n')) if (line.startsWith('func Test')) n++
    }
    return n
  })()

  const packages = readdirSync(join(BACKEND, 'internal'), { withFileTypes: true })
    .filter((e) => e.isDirectory()).length

  return { migrations, routes, testFuncs, packages }
}

/**
 * The claims README makes, as regexes with a capture group holding the number.
 *
 * Kept next to the facts they check so a new header line has an obvious home.
 */
const CLAIMS = [
  { key: 'migrations', label: 'migrations', re: /\((\d+) migrations · schema v\d+\)/ },
  { key: 'routes', label: 'HTTP routes', re: /\*\*(\d+) HTTP routes\*\*/ },
  { key: 'testFuncs', label: 'test functions', re: /\*\*(\d+)\*\* in \d+ packages/ },
  { key: 'packages', label: 'test packages', re: /\*\*\d+\*\* in (\d+) packages/ },
]

/** Compare the README against the tree. Returns a list of drift findings. */
export function findDrift(readmeText, facts) {
  const findings = []
  for (const claim of CLAIMS) {
    const m = claim.re.exec(readmeText)
    if (!m) {
      // A missing claim is drift too: the line was reworded and the check would
      // silently stop guarding anything.
      findings.push({ kind: 'missing', label: claim.label })
      continue
    }
    const documented = Number(m[1])
    if (documented !== facts[claim.key]) {
      findings.push({
        kind: 'mismatch',
        label: claim.label,
        documented,
        actual: facts[claim.key],
      })
    }
  }
  return findings
}

function main() {
  const json = process.argv.includes('--json')

  // `--self-test` proves the rules still fire. A gate that cannot fail is not a
  // gate, so this asserts both directions before trusting the live result.
  if (process.argv.includes('--self-test')) {
    const facts = actualFacts()
    const good = findDrift(
      `Go (${facts.migrations} migrations · schema v16) **${facts.routes} HTTP routes** ` +
        `**${facts.testFuncs}** in ${facts.packages} packages`,
      facts,
    )
    const stale = findDrift('Go (11 migrations · schema v11) **84 HTTP routes** **370** in 11 packages', facts)
    const missing = findDrift('no claims in this text at all', facts)

    // All four claims are wrong in the stale text: migrations, routes, test
    // functions, and the package count.
    const pass = good.length === 0 && stale.length === CLAIMS.length && missing.length === CLAIMS.length
    console.log(`  correct text   -> ${good.length} finding(s), want 0`)
    console.log(`  stale text     -> ${stale.length} finding(s), want ${CLAIMS.length}`)
    console.log(`  no claims text -> ${missing.length} finding(s), want ${CLAIMS.length}`)
    console.log(pass ? 'docs:verify self-test: OK' : 'docs:verify self-test: FAILED')
    process.exit(pass ? 0 : 1)
  }

  const facts = actualFacts()
  const findings = findDrift(readFileSync(README, 'utf8'), facts)

  if (json) {
    console.log(JSON.stringify({ facts, findings }, null, 2))
    process.exit(findings.length === 0 ? 0 : 1)
  }

  console.log(
    `tree: ${facts.migrations} migrations · ${facts.routes} routes · ` +
      `${facts.testFuncs} test functions in ${facts.packages} packages`,
  )

  if (findings.length === 0) {
    console.log('docs:verify: OK — README matches the tree.')
    process.exit(0)
  }

  for (const f of findings) {
    if (f.kind === 'missing') {
      console.error(`  ✗ ${f.label}: README no longer contains the claim this check looks for`)
    } else {
      console.error(
        `  ✗ ${f.label}: README says ${f.documented}, tree has ${f.actual} ` +
          `(in ${relative(ROOT, README)})`,
      )
    }
  }
  console.error('docs:verify: drift found — update the README numbers.')
  process.exit(1)
}

main()
