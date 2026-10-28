import type { InputHTMLAttributes } from "react"

type Props = InputHTMLAttributes<HTMLInputElement> & { label: string }

export function Field({ label, className = "", ...props }: Props) {
  return (
    <label className="flex flex-col gap-1.5 text-sm font-medium text-ink">
      <span>{label}</span>
      <input
        className={`rounded-md border border-border bg-surface px-3 py-2.5 font-normal text-ink placeholder:text-ink/40 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/30 ${className}`}
        {...props}
      />
    </label>
  )
}
