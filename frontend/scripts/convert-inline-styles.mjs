/**
 * VEYLIX — convert token-coverable inline styles to token-backed classes (D-059).
 *
 *   node scripts/convert-inline-styles.mjs --dry   # show what would change
 *   node scripts/convert-inline-styles.mjs         # write
 *
 * Only patterns with an EXACT token class are converted, and only when the inline
 * style contains nothing else — so the rendered result is identical by construction
 * rather than by hope. Anything with an extra property (a `gap`, a `maxWidth`) is
 * left alone and reported, because a half-converted style is how a layout breaks.
 */
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const DRY = process.argv.includes('--dry')
const ROOT = 'src'

/** [inline-body pattern, className] — the body must match this and ONLY this. */
const RULES = [
  [/^fontSize:\s*12$/, 't-sm'],
  [/^fontSize:\s*12\.5$/, 't-sm2'],
  [/^fontSize:\s*13$/, 't-base'],
  [/^fontSize:\s*11\.5$/, 't-xs'],
  [/^margin:\s*'10px 0'$/, 'rhythm'],
  [/^margin:\s*'12px 0'$/, 'rhythm-loose'],
  [/^marginTop:\s*12$/, 'flow-top-3'],
  [/^marginTop:\s*10$/, 'flow-top'],
  [/^marginTop:\s*0$/, 'flow-top-0'],
  [/^textDecoration:\s*'none'$/, 'no-underline'],
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

let converted = 0
let skipped = 0
const touched = new Map()

for (const file of walk(ROOT)) {
  const before = readFileSync(file, 'utf8')
  let after = before

  // `className="a b" style={{ X }}`  →  `className="a b c"`
  after = after.replace(
    /className=(\{?[`"'])([^`"']*)\1(\s*)style=\{\{\s*([^}]+?)\s*\}\}/g,
    (whole, q, cls, gap, body) => {
      const trimmed = body.trim()
      const rule = RULES.find(([re]) => re.test(trimmed))
      if (!rule) {
        skipped++
        return whole
      }
      const next = `${cls} ${rule[1]}`.trim()
      converted++
      return `className=${q}${next}${q}${gap}`
    },
  )

  // A style-only element: `style={{ X }}` → `className="c"`, but ONLY when the
  // element has no className yet. The first pass handles elements that already do;
  // this one replaces `style` in place, and adding a second `className` alongside an
  // existing one produced a duplicate JSX attribute (caught by tsc).
  after = after.replace(
    /(<[A-Za-z][\w.]*)((?:\s+[A-Za-z][\w:-]*=(?:"[^"]*"|\{[^}]*\}))*)\s*style=\{\{\s*([^}]+?)\s*\}\}/g,
    (whole, tag, attrs, body) => {
      const trimmed = body.trim()
      const rule = RULES.find(([re]) => re.test(trimmed))
      if (!rule) return whole
      if (/\bclassName=/.test(attrs)) return whole // handled (or refused) above
      converted++
      return `${tag}${attrs} className="${rule[1]}"`
    },
  )

  if (after !== before) {
    touched.set(file, (before.match(/style=\{\{/g) ?? []).length)
    if (!DRY) writeFileSync(file, after)
  }
}

console.log(`${DRY ? '(dry run) ' : ''}converted ${converted} site(s) across ${touched.size} file(s); ${skipped} left with extra properties`)
if (DRY) for (const [f, n] of touched) console.log(`  ${f}  (${n} inline before)`)
