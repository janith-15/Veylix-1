import type { SVGProps } from 'react'

/**
 * VEYLIX VPN brand mark — shield with a V, lock core and circuit wings.
 * Colours are driven by CSS variables so the mark adapts to every theme:
 *   --logo-a      left / primary side of the V (theme primary)
 *   --logo-b      right / secondary side of the V (theme secondary)
 *   --logo-metal  shield + lock metal
 */
export function VeylixLogo({
  size = 32,
  showText = false,
  ...rest
}: { size?: number; showText?: boolean } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      width={showText ? size * 3.4 : size}
      height={size}
      viewBox={showText ? '0 0 340 100' : '0 0 100 100'}
      role="img"
      aria-label="VEYLIX VPN"
      {...rest}
    >
      <defs>
        <linearGradient id="vx-left" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="var(--logo-a, #6366f1)" />
          <stop offset="100%" stopColor="var(--logo-a-2, #4f46e5)" />
        </linearGradient>
        <linearGradient id="vx-right" x1="1" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--logo-b, #06b6d4)" />
          <stop offset="100%" stopColor="var(--logo-b-2, #0284c7)" />
        </linearGradient>
        <linearGradient id="vx-metal" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="var(--logo-metal-2, #f1f5f9)" />
          <stop offset="50%" stopColor="var(--logo-metal, #94a3b8)" />
          <stop offset="100%" stopColor="var(--logo-metal-2, #f1f5f9)" />
        </linearGradient>
      </defs>

      <g transform={showText ? 'translate(4 6) scale(0.86)' : undefined}>
        <path
          d="M50 6 L88 22 V52 C88 72 70 86 50 94 C30 86 12 72 12 52 V22 Z"
          fill="none"
          stroke="url(#vx-metal)"
          strokeWidth="3.2"
          strokeLinejoin="round"
        />
        <path
          d="M50 12 L82 26 V52 C82 69 67 81 50 88 C33 81 18 69 18 52 V26 Z"
          fill="var(--logo-shield-fill, rgba(99,102,241,0.08))"
        />

        <g stroke="url(#vx-left)" strokeWidth="1.6" fill="none" opacity="0.85">
          <path d="M6 44 h10 l6 -5 h8" />
          <path d="M4 55 h8 l7 6 h9" />
          <path d="M10 66 h9 l5 -5" />
        </g>
        <g stroke="url(#vx-right)" strokeWidth="1.6" fill="none" opacity="0.85">
          <path d="M94 44 h-10 l-6 -5 h-8" />
          <path d="M96 55 h-8 l-7 6 h-9" />
          <path d="M90 66 h-9 l-5 -5" />
        </g>

        <path d="M26 30 L47 74 L50 80 L50 60 L35 30 Z" fill="url(#vx-left)" />
        <path d="M74 30 L53 74 L50 80 L50 60 L65 30 Z" fill="url(#vx-right)" />

        <g transform="translate(50 46)">
          <path
            d="M-7 -4 v-3 a7 7 0 0 1 14 0 v3"
            fill="none"
            stroke="url(#vx-metal)"
            strokeWidth="2.4"
            strokeLinecap="round"
          />
          <rect x="-10" y="-4" width="20" height="16" rx="3.2" fill="url(#vx-metal)" opacity="0.95" />
          <circle cx="0" cy="3.5" r="2.1" fill="var(--logo-lock, #1e293b)" />
        </g>

        <circle cx="50" cy="18" r="3" fill="url(#vx-left)" />
        <circle cx="50" cy="84" r="3" fill="url(#vx-right)" />
      </g>

      {showText && (
        <g>
          <text
            x="106"
            y="52"
            fontFamily="var(--font-family)"
            fontSize="36"
            fontWeight="800"
            letterSpacing="2"
            fill="var(--text-strong, #0f172a)"
          >
            VEYLIX
          </text>
          <text
            x="110"
            y="76"
            fontFamily="var(--font-family)"
            fontSize="14"
            fontWeight="600"
            letterSpacing="7"
            fill="var(--logo-b, #06b6d4)"
          >
            VPN
          </text>
        </g>
      )}
    </svg>
  )
}
