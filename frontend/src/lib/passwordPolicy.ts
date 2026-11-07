/**
 * Client-side mirror of the backend password policy.
 *
 * Single source of truth: `backend/internal/auth/password.go`
 * (`MinPasswordLength`, `ValidatePasswordPolicy`). Keeping the numbers in one
 * place per side means the UI can never say "Looks good ✓" for a password the
 * server is about to reject (finding F-015).
 *
 * This module is a *hint* only — the server is always the authority.
 */

/** Minimum accepted password length, in characters (runes on the server). */
export const MIN_PASSWORD_LENGTH = 12

/** Maximum accepted length; matches the server's DoS guard. */
export const MAX_PASSWORD_LENGTH = 512

/** Common passwords rejected even though they are long enough. */
const BLOCKLIST: ReadonlySet<string> = new Set([
  'password1234',
  'password12345',
  'passwordpassword',
  'qwertyuiop123',
  '123456789012',
  '1234567890123',
  'abcdefghijkl',
  'iloveyou1234',
  'administrator',
  'veylixadmin12',
])

function isSingleRepeatedCharacter(value: string): boolean {
  return value.length > 0 && value.split('').every((c) => c === value[0])
}

function isStraightSequence(value: string): boolean {
  if (value.length < 2) return false
  let ascending = true
  let descending = true
  for (let i = 1; i < value.length; i++) {
    const delta = value.charCodeAt(i) - value.charCodeAt(i - 1)
    if (delta !== 1) ascending = false
    if (delta !== -1) descending = false
  }
  return ascending || descending
}

/**
 * Returns the list of policy violations for `password`.
 * An empty array means the password satisfies the client-side policy.
 */
export function passwordViolations(password: string): string[] {
  const problems: string[] = []
  if (password.length < MIN_PASSWORD_LENGTH) {
    problems.push(`must be at least ${MIN_PASSWORD_LENGTH} characters`)
  }
  if (password.length > MAX_PASSWORD_LENGTH) {
    problems.push(`must be at most ${MAX_PASSWORD_LENGTH} characters`)
  }
  if (isSingleRepeatedCharacter(password)) {
    problems.push('must not be a single repeated character')
  }
  if (isStraightSequence(password)) {
    problems.push('must not be a simple sequence')
  }
  if (BLOCKLIST.has(password.toLowerCase())) {
    problems.push('is too common')
  }
  return problems
}

/**
 * A short hint for the field, shown under the input while typing.
 * Mirrors `passwordViolations` so the message and the rule never disagree.
 */
export function passwordHint(password: string): string {
  if (password.length === 0) return `Use at least ${MIN_PASSWORD_LENGTH} characters`
  const problems = passwordViolations(password)
  if (problems.length === 0) return 'Looks good ✓'
  if (password.length < MIN_PASSWORD_LENGTH) {
    const left = MIN_PASSWORD_LENGTH - password.length
    return `${left} more character${left === 1 ? '' : 's'} needed`
  }
  return `Password ${problems[0]}`
}
