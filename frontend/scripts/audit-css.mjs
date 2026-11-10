/**
 * VEYLIX — CSS integrity audit (`npm run audit:css`).
 *
 * WHY THIS EXISTS
 * ---------------
 * On 2026-11-08 the Offers & Orders screen rendered effectively unstyled in the
 * browser while every file *looked* fine on disk. The cause was three
 * PowerShell multi-part writes that had silently truncated `offers.css`:
 *
 *   1. `.of-spec__v {` opened, then the next line was a comment — the closing
 *      brace and three declarations were gone, and the *same* three declarations
 *      reappeared later as orphans after a stray `}`.
 *   2. `.of-table td.of-num {` opened, then a comment — closing brace gone.
 *   3. A `@media (pointer: coarse)` block ended, then `white-space: nowrap;`
 *      and a stray `}` floated at depth 0.
 *
 * Balanced brace *counts* (92/92) hid all of it, and `tsc`/`oxlint`/`vite build`
 * all passed: CSS is not type-checked and a browser silently drops rules it
 * cannot parse. The visible damage was that 44 of 69 selectors — `.of-grid`,
 * `.of-toolbar`, `.of-table`, `.of-cmp`, `.of-filters` — never reached the page.
 *
 * This script catches exactly those shapes:
 *   - brace depth going negative (a stray `}`)
 *   - depth not returning to 0 at end of file
 *   - a declaration at depth 0 (an orphan property outside any rule)
 *   - a block that opens and reaches the next top-level construct unclosed
 *
 * It does NOT try to be a CSS parser. It is a truncation detector, which is the
 * failure mode this repository actually produced.
 *
 * Usage:
 *   node scripts/audit-css.mjs              audit every .css under src/
 *   node scripts/audit-css.mjs --json       machine-readable output
 *   node scripts/audit-css.mjs --self-test  prove the rules still fire
 *
 * Exit code 1 on any finding so it can gate CI.
 */
import { readdirSync, readFileSync, statSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { tmpdir } from 'node:os'
import { fileURLToPath } from 'node:url'

const SRC = fileURLToPath(new URL('../src/', import.meta.url))
const ROOT = fileURLToPath(new URL('../', import.meta.url))

/** Property-looking line at depth 0 — `color: red;` outside any rule. */
const DECLARATION = /^[-a-z][-a-z0-9]*\s*:\s*\S/i

/**
 * Walk a stylesheet and return findings. Line-based, deliberately: the failure
 * it detects is a *line-shaped* truncation, and a line-based check names the
 * exact line a human has to look at.
 *
 * `@media`/`@supports`/`@layer`/`@container` open an extra nesting level, which
 * is why depth is counted rather than "am I inside a rule" being a boolean.
 */
function auditSource(text) {
  const findings = []
  const lines = text.split(/\r?\n/)
  let depth = 0
  let inComment = false
  let blockStartLine = 0
  let blockIsAtRule = false
  let sawDeclarationInBlock = false

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i]
    const line = raw.trim()
    const lineNo = i + 1

    if (line === '') continue

    if (inComment) {
      if (line.includes('*/')) inComment = false
      continue
    }
    if (line.startsWith('/*')) {
      if (!line.includes('*/')) inComment = true
      continue
    }
    // `//` is not valid CSS but appears in this repo's comments; ignore it so a
    // comment cannot be mistaken for a declaration.
    if (line.startsWith('//')) continue

    const opens = (raw.match(/\{/g) ?? []).length
    const closes = (raw.match(/\}/g) ?? []).length

    if (depth === 0 && opens === 0 && closes === 0) {
      if (DECLARATION.test(line)) {
        findings.push({
          line: lineNo,
          rule: 'orphan-declaration',
          detail: `declaration outside any rule: "${line}"`,
        })
      }
      continue
    }

    // A stray `}` is one that closes more blocks than are currently open.
    // Compare against `depth + opens`, not `depth`: a self-contained one-line
    // rule (`[data-ui='flat'] .x { border-radius: 0; }`) opens and closes on
    // the same line and is perfectly valid.
    if (closes > 0 && depth + opens - closes < 0) {
      findings.push({
        line: lineNo,
        rule: 'stray-closing-brace',
        detail: `"${line}" closes a block that was never opened`,
      })
    }

    if (opens > 0) {
      // A new block opening while another is open is only *suspicious* when the
      // open block is a plain selector rule holding declarations. A nested
      // block inside `@media`/`@supports`/`@layer`/`@container` is valid CSS and
      // must not be reported, so at-rule nesting is tracked separately.
      const opensAtRule = /^\s*@(media|supports|layer|container|scope|starting-style|keyframes|-webkit-keyframes|font-face|page|property|counter-style|font-feature-values)\b/i.test(raw)
      if (depth === 0) {
        blockStartLine = lineNo
        blockIsAtRule = opensAtRule
        sawDeclarationInBlock = false
      } else if (sawDeclarationInBlock && !blockIsAtRule) {
        // The `.of-spec__v` shape: a plain rule has declarations, then a new
        // rule opens without the previous one closing.
        findings.push({
          line: lineNo,
          rule: 'unclosed-block-before-selector',
          detail: `a new block opens at line ${lineNo} while the rule started at line ${blockStartLine} is still open`,
        })
      }
      // Entering a nested block clears the "already has declarations" flag for
      // the new inner block.
      if (!opensAtRule && depth > 0) sawDeclarationInBlock = false
    }

    if (depth > 0 && opens === 0) sawDeclarationInBlock = true

    depth += opens - closes
    if (depth < 0) depth = 0
    if (depth === 0) sawDeclarationInBlock = false
  }

  if (depth !== 0) {
    findings.push({
      line: lines.length,
      rule: 'unclosed-at-eof',
      detail: `file ends at brace depth ${depth}; a block was never closed`,
    })
  }

  /*
   * Comment balance.
   *
   * A second truncation shape was found the same day: an edit landed a CSS rule
   * whose opening comment marker was written at the insert point while the
   * comment body and the rule itself landed further down the file. The rule
   * still parsed on its own, so the brace walk above saw nothing wrong, but the
   * comment it sat inside had swallowed 120 lines - Vite answered 500 for the
   * whole stylesheet and a screen rendered unstyled with no error in any file.
   *
   * Counting the two comment delimiters catches that in one pass, on disk,
   * before a browser is involved.
   */
  const opensComments = (text.match(/\/\*/g) ?? []).length
  const closesComments = (text.match(/\*\//g) ?? []).length
  if (opensComments !== closesComments) {
    findings.push({
      line: lines.length,
      rule: 'unbalanced-comment',
      detail: `${opensComments} opening vs ${closesComments} closing comment delimiters - a comment is not terminated`,
    })
  }

  return findings
}

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    const st = statSync(full)
    if (st.isDirectory()) walk(full, out)
    else if (entry.endsWith('.css')) out.push(full)
  }
  return out
}

function runAudit() {
  const files = walk(SRC).sort()
  const results = []
  for (const file of files) {
    const findings = auditSource(readFileSync(file, 'utf8'))
    if (findings.length > 0) {
      results.push({ file: relative(ROOT, file).split(sep).join('/'), findings })
    }
  }
  return { filesChecked: files.length, results }
}

function printReport({ filesChecked, results }) {
  if (results.length === 0) {
    console.log(`CSS integrity audit: OK - ${filesChecked} stylesheets parsed, no truncation found.`)
    return
  }
  for (const { file, findings } of results) {
    for (const f of findings) {
      console.error(`${file}:${f.line}  [${f.rule}]  ${f.detail}`)
    }
  }
  console.error(`\nCSS integrity audit: FAIL - ${results.length} file(s) with findings.`)
}

/** Prove the detectors still fire, on synthetic input, with no files touched. */
function selfTest() {
  const cases = [
    {
      name: 'clean stylesheet',
      src: '.a {\n  color: red;\n}\n',
      expect: 0,
    },
    {
      name: 'orphan declaration after a closed block (.of-table shape)',
      src: '@media (pointer: coarse) {\n  .a {\n    min-height: 44px;\n  }\n}\n\n  white-space: nowrap;\n}\n',
      expect: 2,
    },
    {
      name: 'unclosed block then a selector (.of-spec__v shape)',
      src: '.spec {\n  font-size: 1rem;\n\n/* comment */\n.next {\n  color: red;\n}\n',
      expect: 2,
    },
    {
      name: 'unclosed block at EOF',
      src: '.a {\n  color: red;\n',
      expect: 1,
    },
    {
      name: 'one-line rule is not a stray brace',
      src: "[data-ui='flat'] .x { border-radius: 0; }\n",
      expect: 0,
    },
    {
      name: 'nested at-rule is not a finding',
      src: '@media (max-width: 720px) {\n  .a {\n    color: red;\n  }\n}\n',
      expect: 0,
    },
    {
      name: '@keyframes nested from/to is not a finding',
      src: '@keyframes fade {\n  from {\n    opacity: 0;\n  }\n  to {\n    opacity: 1;\n  }\n}\n',
      expect: 0,
    },
    {
      name: 'comments are ignored',
      src: '/* .a { color: red; */\n.b {\n  color: blue;\n}\n',
      expect: 0,
    },
    {
      name: 'unbalanced comment is a finding',
      // A rule whose opening comment marker is at the top but never closes.
      src: ['.a {', '  color: red;', '}', '/*', ' * orphaned comment body', ''].join('\n') + '\n.b {\n  color: blue;\n}\n',
      expect: 1,
    },
  ]

  let failed = 0
  for (const c of cases) {
    const findings = auditSource(c.src)
    // A truncation usually trips more than one rule (the missing brace causes a
    // later stray one), so assert the expected count exactly.
    const ok = findings.length === c.expect
    if (!ok) {
      failed++
      console.error(`  x ${c.name}: expected ${c.expect} finding(s), got ${findings.length}`)
      for (const f of findings) console.error(`      ${f.rule}: ${f.detail}`)
    } else {
      console.log(`  ok ${c.name}`)
    }
  }

  // End-to-end: a temp directory, never src/ and never a real file.
  const tmp = mkdtempSync(join(tmpdir(), 'veylix-css-audit-'))
  try {
    writeFileSync(join(tmp, 'broken.css'), '.a {\n  color: red;\n\n.b {\n  color: blue;\n}\n', 'utf8')
    writeFileSync(join(tmp, 'ok.css'), '.a {\n  color: red;\n}\n', 'utf8')
    const broken = auditSource(readFileSync(join(tmp, 'broken.css'), 'utf8'))
    const good = auditSource(readFileSync(join(tmp, 'ok.css'), 'utf8'))
    if (broken.length > 0 && good.length === 0) {
      console.log('  ok temp-directory end-to-end write/read')
    } else {
      failed++
      console.error('  x temp-directory end-to-end write/read')
    }
  } finally {
    rmSync(tmp, { recursive: true, force: true })
  }

  if (failed > 0) {
    console.error(`\nCSS audit self-test: FAIL - ${failed} case(s) failed.`)
    process.exit(1)
  }
  console.log('\nCSS audit self-test: OK - every detector fires and no false positives.')
}

const args = process.argv.slice(2)
if (args.includes('--self-test')) {
  selfTest()
} else {
  const report = runAudit()
  if (args.includes('--json')) {
    console.log(JSON.stringify(report, null, 2))
  } else {
    printReport(report)
  }
  if (report.results.length > 0) process.exit(1)
}

