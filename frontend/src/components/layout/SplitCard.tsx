import type { ReactNode } from "react"

type Props = { title: string; text: string; children: ReactNode }

export function SplitCard({ title, text, children }: Props) {
  return (
    <section className="grid overflow-hidden rounded-lg border border-border bg-surface md:grid-cols-2">
      <div className="bg-brand-900 p-10 text-white">
        <div className="h-1 w-12 bg-accent" />
        <h1 className="mt-6 text-3xl font-semibold leading-tight">{title}</h1>
        <p className="mt-4 max-w-sm text-brand-50/80">{text}</p>
      </div>
      <div className="p-10">{children}</div>
    </section>
  )
}
