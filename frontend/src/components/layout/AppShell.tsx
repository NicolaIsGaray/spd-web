import { Link, Outlet } from "react-router-dom"
import { useSession } from "../../context/SessionContext"
import { initials } from "../../utils/initials"
import { Brand } from "../ui/Brand"

export default function AppShell() {
  const { session } = useSession()
  return (
    <div className="min-h-screen">
      <header className="border-b border-border bg-surface">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <Link to="/salas" aria-label="Ir a mis salas">
            <Brand />
          </Link>
          {session && (
            <div className="flex items-center gap-2.5 rounded-full border border-border py-1 pl-1 pr-4 text-sm font-medium">
              <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand-700 text-xs font-semibold text-white">
                {initials(session.name)}
              </span>
              {session.name}
            </div>
          )}
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-6 py-10">
        <Outlet />
      </main>
    </div>
  )
}
