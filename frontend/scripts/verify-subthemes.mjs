/**
 * Count sub-themes per preset by importing the real modules.
 *
 * Runs the actual `subthemes.ts` through Vite's SSR transform so the answer
 * comes from the code the app ships, not from a regex over the source.
 */
import { createServer } from 'vite'

const server = await createServer({
  root: process.cwd(),
  server: { middlewareMode: true },
  appType: 'custom',
  logLevel: 'error',
})

try {
  const mod = await server.ssrLoadModule('/src/theme/subthemes.ts')
  const { SUB_THEMES, ALL_SUB_THEMES } = mod
  let total = 0
  let failed = 0
  const presets = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']

  for (const p of presets) {
    const list = SUB_THEMES[p] ?? []
    const ids = list.map((s) => s.id)
    const dupes = ids.filter((id, i) => ids.indexOf(id) !== i)
    total += list.length
    const ok = list.length === 10 && dupes.length === 0
    if (!ok) failed++
    console.log(
      `${ok ? 'PASS' : 'FAIL'}  ${p.padEnd(8)} ${String(list.length).padStart(2)} sub-themes` +
        (dupes.length ? `  DUPLICATES: ${dupes.join(',')}` : ''),
    )
  }

  // A variant with no colours would render an unstyled tile.
  //
  // The contract is narrower than "defines every token": a variant may override
  // ONLY its accents and inherit `--bg-page` / `--text` from its preset — that is
  // how `soft` and the other hand-written variants have always worked, and
  // `ThemeProvider` layers preset → variant in that order. What must always exist
  // is the accent pair, because that is what makes the tile recognisable.
  const broken = ALL_SUB_THEMES.filter((s) => !s.vars?.['--c-primary'] || !s.vars?.['--c-secondary'])
  if (broken.length) {
    failed++
    console.log(
      `FAIL  ${broken.length} sub-themes missing accents: ${broken.slice(0, 5).map((b) => b.id).join(',')}`,
    )
  } else {
    console.log('PASS  every sub-theme defines its accent pair')
  }

  console.log(`\nTOTAL ${total}  (target 60)`)
  console.log(failed === 0 && total === 60 ? 'SUBTHEMES OK' : `SUBTHEMES FAILED (${failed})`)
  process.exitCode = failed === 0 && total === 60 ? 0 : 1
} finally {
  await server.close()
}
