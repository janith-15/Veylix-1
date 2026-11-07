interface CheckboxProps {
  checked: boolean
  onChange: (value: boolean) => void
  label?: string
  disabled?: boolean
}

export function Checkbox({ checked, onChange, label, disabled }: CheckboxProps) {
  return (
    <label className="ui-row" style={{ gap: 8, cursor: disabled ? 'not-allowed' : 'pointer' }}>
      <input
        type="checkbox"
        className="ui-checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label && <span className="ui-muted" style={{ fontSize: 14 }}>{label}</span>}
    </label>
  )
}
