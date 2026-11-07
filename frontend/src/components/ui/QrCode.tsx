import { useMemo } from 'react'
import { encodeQr } from '../../lib/qr'
import './qr.css'

/**
 * A QR code drawn as inline SVG (D-033).
 *
 * - The pattern is generated locally from the panel's own URI, so the credential
 *   never leaves the browser (D-033.2).
 * - It is SVG, not a canvas, so it stays crisp when printed or saved at any size.
 * - An unencodable link shows a readable message instead of a blank square: a QR
 *   that silently fails is worse than no QR (D-033.4).
 *
 * Colours come from the theme tokens, with a forced white background and black
 * modules because a scanner needs contrast — a themed QR that cannot be read
 * would be worse than an unstyled one.
 */
export function QrCode({
  value,
  size = 220,
  label,
  className,
}: {
  value: string
  size?: number
  /** Accessible description; the link itself is announced by its own field. */
  label: string
  className?: string
}) {
  const encoded = useMemo(() => {
    if (!value) return { error: 'No link to encode yet.' as string, matrix: null }
    try {
      return { error: null, matrix: encodeQr(value) }
    } catch (err) {
      return { error: err instanceof Error ? err.message : 'Could not encode this link', matrix: null }
    }
  }, [value])

  if (!encoded.matrix) {
    return (
      <div className={`qr qr--error ${className ?? ''}`} role="img" aria-label={label} data-testid="qr-error">
        <span>{encoded.error}</span>
      </div>
    )
  }

  const { matrix } = encoded
  // A 4-module quiet zone is part of the QR spec: without it many scanners fail.
  const quiet = 4
  const total = matrix.size + quiet * 2
  const path: string[] = []
  for (let y = 0; y < matrix.size; y++) {
    for (let x = 0; x < matrix.size; x++) {
      if (matrix.modules[y][x]) path.push(`M${x + quiet} ${y + quiet}h1v1h-1z`)
    }
  }

  return (
    <div className={`qr ${className ?? ''}`} data-testid="qr-code">
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${total} ${total}`}
        role="img"
        aria-label={label}
        shapeRendering="crispEdges"
      >
        <rect width={total} height={total} fill="#ffffff" />
        <path d={path.join('')} fill="#000000" />
      </svg>
    </div>
  )
}
