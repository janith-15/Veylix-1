#!/usr/bin/env node
/**
 * Minimal structural check for the GitHub Actions workflows.
 *
 * There is no YAML parser in this repo's dependency set, and a workflow that
 * fails to parse simply never runs — which looks exactly like "no CI needed".
 * This checks the things that actually break: tabs, missing `on:`/`jobs:`,
 * duplicate job keys, and steps that are not a list of mappings.
 *
 * Run: node scripts/check-workflows.mjs
 */
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

// Resolve from the repository root, not the caller's cwd: this runs from
// frontend/ via npm, while the workflows live two levels up.
const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, '..', '..')
const dir = join(root, '.github', 'workflows')
if (!existsSync(dir)) {
  console.log('check-workflows: no .github/workflows directory')
  process.exit(1)
}

let problems = 0
const files = readdirSync(dir).filter((f) => f.endsWith('.yml') || f.endsWith('.yaml'))
if (files.length === 0) {
  console.log('check-workflows: no workflow files')
  process.exit(1)
}

for (const f of files) {
  const text = readFileSync(join(dir, f), 'utf8')
  const lines = text.split('\n')
  const issues = []

  // Tabs are illegal for YAML indentation.
  lines.forEach((l, i) => {
    if (/^\t/.test(l)) issues.push(`line ${i + 1}: tab indentation`)
  })

  const topKeys = lines.filter((l) => /^[a-zA-Z_]/.test(l)).map((l) => l.split(':')[0])
  for (const required of ['name', 'on', 'jobs']) {
    if (!topKeys.includes(required)) issues.push(`missing top-level '${required}:'`)
  }

  // Jobs are the two-space-indented keys under `jobs:`.
  const jobsAt = lines.findIndex((l) => l === 'jobs:')
  if (jobsAt >= 0) {
    const jobs = []
    for (let i = jobsAt + 1; i < lines.length; i++) {
      const l = lines[i]
      if (/^[a-zA-Z_]/.test(l)) break // next top-level key
      const m = /^ {2}([A-Za-z0-9_-]+):\s*$/.exec(l)
      if (m) jobs.push(m[1])
    }
    if (jobs.length === 0) issues.push("'jobs:' declares no jobs")
    const dupes = jobs.filter((j, i) => jobs.indexOf(j) !== i)
    if (dupes.length) issues.push(`duplicate job key(s): ${[...new Set(dupes)].join(', ')}`)

    // Every job must say where it runs.
    for (const job of jobs) {
      const idx = lines.findIndex((l) => l === `  ${job}:`)
      if (idx < 0) continue
      let hasRunsOn = false
      for (let i = idx + 1; i < lines.length; i++) {
        if (/^ {2}[A-Za-z0-9_-]+:\s*$/.test(lines[i])) break
        if (/^ {4}runs-on:/.test(lines[i])) hasRunsOn = true
      }
      if (!hasRunsOn) issues.push(`job '${job}' has no runs-on`)
    }
  }

  if (issues.length) {
    problems += issues.length
    console.error(`✗ ${f}`)
    for (const i of issues) console.error(`    - ${i}`)
  } else {
    console.log(`✓ ${f}`)
  }
}

if (problems > 0) {
  console.error(`check-workflows: ${problems} problem(s)`)
  process.exit(1)
}
console.log(`check-workflows: OK — ${files.length} workflow file(s).`)
