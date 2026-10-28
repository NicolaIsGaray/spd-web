import { Icon, type IconName } from "../ui/Icon"

type Props = { active: boolean; icon: IconName; title: string; text: string; onClick: () => void }

export function ModeCard({ active, icon, title, text, onClick }: Props) {
  return (
    <button onClick={onClick} aria-pressed={active}
      className={`flex items-center gap-4 rounded-lg border p-4 text-left transition-colors ${
        active ? "border-brand-700 bg-brand-50" : "border-border bg-surface hover:border-brand-500"
      }`}>
      <span className="flex h-10 w-10 items-center justify-center rounded-md bg-brand-700 text-white">
        <Icon name={icon} />
      </span>
      <span>
        <strong className="block text-brand-700">{title}</strong>
        <small className="text-ink/60">{text}</small>
      </span>
    </button>
  )
}
