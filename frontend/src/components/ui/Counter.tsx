type Props = {
  label: string
  hint: string
  value: number
  min: number
  max: number
  onChange: (value: number) => void
}

const step = "h-8 w-8 rounded-md border border-border text-brand-700 hover:bg-brand-50 disabled:opacity-40"

export function Counter({ label, hint, value, min, max, onChange }: Props) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-md border border-border p-4">
      <div>
        <strong className="block text-sm">{label}</strong>
        <small className="text-ink/60">{hint}</small>
      </div>
      <div className="flex items-center gap-3">
        <button type="button" className={step} aria-label={`Restar: ${label}`}
          disabled={value <= min} onClick={() => onChange(value - 1)}>−</button>
        <strong className="w-6 text-center">{value}</strong>
        <button type="button" className={step} aria-label={`Sumar: ${label}`}
          disabled={value >= max} onClick={() => onChange(value + 1)}>+</button>
      </div>
    </div>
  )
}
