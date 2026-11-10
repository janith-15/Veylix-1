/**
 * VEYLIX — report how many theme-bypassing inline styles remain (D-059).
 *
 *   node scripts/check-inline-styles.mjs           # summary
 *   node scripts/check-inline-styles.mjs --list    # every site
 *
 * A `style={{ fontSize: 12.5 }}` ignores `--font-size-scale`, so Theme Studio cannot
 * reach it and a sub-theme change silently skips it. The remaining ones are counted
 * by CATEGORY, and the count is the gate: it must not grow.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

const ROOT = 'src'
const LIST = process.argv.includes('--list')

/** Properties with a token-backed class. Everything else still needs one. */
const COVERED = [
  [/fontSize:\s*12\b/, 'fontSize 12 → .t-sm'],
  [/fontSize:\s*12\.5/, 'fontSize 12.5 → .t-sm2'],
  [/fontSize:\s*13\b/, 'fontSize 13 → .t-base'],
  [/fontSize:\s*11\.5/, 'fontSize 11.5 → .t-xs'],
  [/margin:\s*'10px 0'/, "margin '10px 0' → .rhythm"],
  [/margin:\s*'12px 0'/, "margin '12px 0' → .rhythm-loose"],
  [/marginTop:\s*12\b/, 'marginTop 12 → .flow-top-3'],
  [/marginTop:\s*10\b/, 'marginTop 10 → .flow-top'],
  [/textDecoration:\s*'none'/, "textDecoration none → .no-underline"],
]

function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...walk(p))
    else if (/\.tsx$/.test(name)) out.push(p)
  }
  return out
}

const files = walk(ROOT)
const groups = new Map()
const sites = []

for (const file of files) {
  const lines = readFileSync(file, 'utf8').split('\n')
  lines.forEach((line, i) => {
    const m = line.match(/style=\{\{([^}]*)/)
    if (!m) return
    const body = m[1].trim()
    const covered = COVERED.find(([re]) => re.test(body))
    const key = covered ? 'coverable: ' + covered[1] : 'needs a new class'
    groups.set(key, (groups.get(key) ?? 0) + 1)
    sites.push(`${file}:${i + 1}  ${body.slice(0, 70)}`)
  })
}

const total = [...groups.values()].reduce((a, b) => a + b, 0)
console.log(`${files.length} files · ${total} inline style sites`)
console.log('')
for (const [k, n] of [...groups.entries()].sort((a, b) => b[1] - a[1])) {
  console.log(`  ${String(n).padStart(3)}  ${k}`)
}
if (LIST) {
  console.log('')
  for (const s of sites) console.log('  ' + s)
}
