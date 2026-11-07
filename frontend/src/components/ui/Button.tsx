import type { ButtonHTMLAttributes, ReactNode } from 'react'

export type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger'
export type ButtonSize = 'sm' | 'md' | 'lg'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  loading?: boolean
  leading?: ReactNode
}

export function Button({
  variant = 'primary',
  size = 'md',
  loading = false,
  leading,
  children,
  disabled,
  className = '',
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      aria-busy={loading}
      className={`ui-btn ui-btn--${variant} ui-btn--${size} ${className}`}
    >
      {loading ? <span className="ui-spinner ui-spinner--sm" aria-hidden="true" /> : leading}
      {children}
    </button>
  )
}
