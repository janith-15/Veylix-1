interface CheckboxProps {
  checked: boolean
  onChange: (value: boolean) => void
  label?: string
  disabled?: boolean
}

export function Checkbox({ checked, onChange, label, disabled }: CheckboxProps) {
  return (
    /*
     * D-059: `gap`, cursor and label size are layout facts shared by every
     * checkbox, so they live in `.ui-check` in the stylesheet. A style prop here
     * was copied onto the DOM of every checkbox in the application.
     */
    <label className="ui-row ui-check" data-disabled={disabled ? 'true' : undefined}>
      <input
        type="checkbox"
        className="ui-checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label && <span className="ui-muted ui-check__label">{label}</span>}
    </label>
  )
}
