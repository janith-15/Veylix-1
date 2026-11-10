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

/**
 * A byte unit the reader can pin, or `auto` to let the magnitude decide.
 *
 * `auto` is the default everywhere because it is the honest reading of a single
 * figure: "33.3 GB" tells a customer more than "34,127 MB". Pinning a unit is a
 * CHART affordance first — an axis whose ticks jump between `900 MB` and
 * `1.1 GB` is harder to compare than one that stays in GB throughout.
 */
export type ByteUnit = 'auto' | 'B' | 'KB' | 'MB' | 'GB' | 'TB'

/** The units a reader may pin, in the order the picker lists them. */
export const BYTE_UNITS: readonly ByteUnit[] = ['auto', 'GB', 'MB', 'TB', 'KB', 'B']

/** Bytes per unit, for the pinned conversions. `auto` is not in the table. */
const UNIT_SCALE: Record<Exclude<ByteUnit, 'auto'>, number> = {
  B: 1,
  KB: 1024,
  MB: 1024 ** 2,
  GB: 1024 ** 3,
  TB: 1024 ** 4,
}

/**
 * Format bytes in a unit the caller chose — for chart axes, bar labels and
 * tooltips, where the same unit must hold across every point in the series.
 *
 * `auto` delegates to `formatBytes`, so a screen that never pins a unit behaves
 * exactly as it did before this existed. That delegation is deliberate: it keeps
 * one definition of "how do we shorten bytes" rather than two that can drift.
 *
 * A pinned unit does NOT shorten further. `2000 MB` stays `2000.0 MB` rather than
 * becoming `2.0 GB`, because a reader who asked for MB is comparing MB.
 */
export function formatBytesIn(bytes: number, unit: ByteUnit = 'auto'): string {
  if (unit === 'auto') return formatBytes(bytes)
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  const scale = UNIT_SCALE[unit]
  const v = bytes / scale
  // Whole bytes carry no meaningful fraction; anything larger gets one decimal
  // until it reaches three digits, where a decimal is noise.
  if (unit === 'B') return `${Math.round(v)} B`
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${unit}`
}

/** The short axis label for a unit, used in a picker trigger. */
export function unitLabel(unit: ByteUnit): string {
  return unit === 'auto' ? 'Auto' : unit
}

/**
 * The default value label for a chart: a grouped number, never a raw float.
 *
 * It lives here rather than beside the charts because a module that exports a
 * component should export only components — a shared constant in `LineChart.tsx`
 * breaks fast refresh for every chart on the page.
 */
export function plainValue(v: number): string {
  return Number.isFinite(v) ? v.toLocaleString() : '—'
}

