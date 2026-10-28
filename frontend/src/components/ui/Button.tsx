import type { ButtonHTMLAttributes, ReactNode } from "react"
import { Icon, type IconName } from "./Icon"

type Variant = "primary" | "secondary" | "accent" | "ghost"

const variants: Record<Variant, string> = {
  primary: "bg-brand-700 text-white hover:bg-brand-900",
  secondary: "border border-border bg-surface text-brand-700 hover:bg-brand-50",
  accent: "bg-accent text-brand-900 hover:bg-accent-strong hover:text-white",
  ghost: "text-brand-700 hover:bg-brand-50",
}

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant
  icon?: IconName
  children: ReactNode
}

export function Button({ variant = "primary", icon, children, className = "", ...props }: Props) {
  return (
    <button
      className={`inline-flex items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${className}`}
      {...props}
    >
      {icon && <Icon name={icon} />}
      {children}
    </button>
  )
}
