import { createContext, useContext, useState, type ReactNode } from "react"

type Session = { name: string } | null
type SessionValue = { session: Session; signIn: (name: string) => void; signOut: () => void }

const SessionContext = createContext<SessionValue | null>(null)

// Sesión simulada: reemplazar por la autenticación real cuando el back la tenga.
export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session>(null)
  const value: SessionValue = {
    session,
    signIn: (name) => setSession({ name }),
    signOut: () => setSession(null),
  }
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}

export function useSession() {
  const context = useContext(SessionContext)
  if (!context) throw new Error("useSession debe usarse dentro de SessionProvider")
  return context
}
