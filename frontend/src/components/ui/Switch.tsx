interface SwitchProps {
  checked: boolean
  onChange: (value: boolean) => void
  label?: string
  disabled?: boolean
}

export function Switch({ checked, onChange, label, disabled }: SwitchProps) {
  return (
    /* D-059: spacing and cursor are shared, so they live in `.ui-switch-row`. */
    <label className="ui-row ui-switch-row" data-disabled={disabled ? 'true' : undefined}>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label || 'toggle'}
        data-on={checked}
        className="ui-switch"
        disabled={disabled}
        onClick={() => onChange(!checked)}
      >
        <span className="ui-switch__thumb" />
      </button>
      {label && (
        <span className="ui-muted t-base">
          {label}
        </span>
      )}
    </label>
  )
}
