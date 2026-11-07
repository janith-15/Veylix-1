/** Humanize SCREAMING_SNAKE → "Screaming Snake". */
export function humanize(value: string): string {
  return value
    .toLowerCase()
    .split('_')
    .map((w) => (w ? w.charAt(0).toUpperCase() + w.slice(1) : w))
    .join(' ')
}

/**
 * Format a price held in MINOR units (cents) for display.
 *
 * Money is carried on the wire as an integer of minor units — never a float —
 * because 0.1 + 0.2 is not 0.3. The conversion to a decimal string happens here,
 * at the very edge, once.
 *
 * `undefined`/`null` currency falls back to the account's own currency code when
 * one is supplied; with neither, the number is shown without a symbol rather than
 * guessed, because a wrong currency symbol on a price is worse than none.
 */
export function formatMoney(
  amountMinor: number,
  currency = '',
  locale?: string,
): string {
  if (!Number.isFinite(amountMinor)) return '—'
  const major = amountMinor / 100
  if (currency) {
    try {
      return new Intl.NumberFormat(locale, {
        style: 'currency',
        currency,
        minimumFractionDigits: major % 1 === 0 ? 0 : 2,
        maximumFractionDigits: 2,
      }).format(major)
    } catch {
      // An unknown ISO code must not throw a render away.
      return `${major.toFixed(2)} ${currency}`
    }
  }
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(major)
}

/** Format bytes into a friendly string. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}
