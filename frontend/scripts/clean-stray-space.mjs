import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

function walk(d) {
  const out = []
  for (const n of readdirSync(d)) {
    const p = join(d, n)
    if (statSync(p).isDirectory()) out.push(...walk(p))
    else if (/\.tsx$/.test(n)) out.push(p)
  }
  return out
}

let n = 0
for (const f of walk('src')) {
  const before = readFileSync(f, 'utf8')
  // The converter produced `className="a b" >` — a stray space before the closing
  // angle bracket. Harmless to JSX, but it is noise in every diff.
  const after = before.replace(/" >/g, '">')
  if (after !== before) {
    n++
    writeFileSync(f, after)
  }
}
console.log('cleaned', n, 'file(s)')
