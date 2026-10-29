import { createContext, useContext, useState, type ReactNode } from "react"

// Sesión del navegador. Sin autenticación en el backend todavía (no hay endpoint de login), así
// que es similar: el nombre va en el header y el userId sirve para las llamadas que necesitan
// identidad (unirse a sala/grupo, ver "mis" salas). Cuando el back la tenga, acá va el token.

export type Session = {
  name: string
  /** Id del usuario en el backend, si el flujo que inició sesión creó/registró uno real. */
  userId?: number
}

type SessionValue = {
  session: Session | null
  signIn: (session: Session) => void
  signOut: () => void
}

const SessionContext = createContext<SessionValue | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const value: SessionValue = {
    session,
    signIn: (next) => setSession(next),
    signOut: () => setSession(null),
  }
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}

export function useSession() {
  const context = useContext(SessionContext)
  if (!context) throw new Error("useSession debe usarse dentro de SessionProvider")
  return context
}
