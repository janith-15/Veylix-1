import { forwardRef, useId, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'

/**
 * Field messages are LINKED to their control, not just drawn next to it.
 *
 * `aria-invalid` alone tells a screen reader that a value is wrong and says nothing
 * about why — the explanation sat in a sibling `<span>` that was never referenced,
 * and the whole app had zero `aria-describedby`. Each hint/error now carries an id
 * and the control points at it, so the message is announced with the field.
 */
function describedBy(hintId: string | null, errorId: string | null, explicit?: string) {
  return [explicit, errorId, hintId].filter(Boolean).join(' ') || undefined
}

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string
  hint?: string
  error?: string
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, hint, error, id, className = '', ...rest },
  ref,
) {
  const autoId = useId()
  const fieldId = id ?? autoId
  const hintId = hint ? `${fieldId}-hint` : null
  const errorId = error ? `${fieldId}-error` : null
  return (
    <div className="ui-field">
      {label && (
        <label className="ui-field__label" htmlFor={fieldId}>
          {label}
        </label>
      )}
      <input
        id={fieldId}
        ref={ref}
        className={`ui-input ${error ? 'ui-input--error' : ''} ${className}`}
        aria-invalid={!!error}
        aria-describedby={describedBy(hintId, errorId, rest['aria-describedby'])}
        {...rest}
      />
      {error ? (
        <span className="ui-field__error" id={errorId ?? undefined} role="alert">
          {error}
        </span>
      ) : (
        hint && (
          <span className="ui-field__hint" id={hintId ?? undefined}>
            {hint}
          </span>
        )
      )}
    </div>
  )
})

export function Textarea(
  props: TextareaHTMLAttributes<HTMLTextAreaElement> & {
    label?: string
    hint?: string
    error?: string
  },
) {
  const autoId = useId()
  const fieldId = props.id ?? autoId
  const { label, hint, error, ...rest } = props
  const hintId = hint ? `${fieldId}-hint` : null
  const errorId = error ? `${fieldId}-error` : null
  return (
    <div className="ui-field">
      {label && (
        <label className="ui-field__label" htmlFor={fieldId}>
          {label}
        </label>
      )}
      <textarea
        id={fieldId}
        className={`ui-input ${error ? 'ui-input--error' : ''}`}
        aria-invalid={!!error}
        aria-describedby={describedBy(hintId, errorId, rest['aria-describedby'])}
        {...rest}
      />
      {error ? (
        <span className="ui-field__error" id={errorId ?? undefined} role="alert">
          {error}
        </span>
      ) : (
        hint && (
          <span className="ui-field__hint" id={hintId ?? undefined}>
            {hint}
          </span>
        )
      )}
    </div>
  )
}

export function Select(
  props: SelectHTMLAttributes<HTMLSelectElement> & {
    children: ReactNode
    label?: string
    hint?: string
    error?: string
  },
) {
  const autoId = useId()
  const fieldId = props.id ?? autoId
  const { label, hint, error, children, ...rest } = props
  const hintId = hint ? `${fieldId}-hint` : null
  const errorId = error ? `${fieldId}-error` : null
  return (
    <div className="ui-field">
      {label && (
        <label className="ui-field__label" htmlFor={fieldId}>
          {label}
        </label>
      )}
      <select
        id={fieldId}
        className="ui-input"
        aria-invalid={!!error}
        aria-describedby={describedBy(hintId, errorId, rest['aria-describedby'])}
        {...rest}
      >
        {children}
      </select>
      {error ? (
        <span className="ui-field__error" id={errorId ?? undefined} role="alert">
          {error}
        </span>
      ) : (
        hint && (
          <span className="ui-field__hint" id={hintId ?? undefined}>
            {hint}
          </span>
        )
      )}
    </div>
  )
}

