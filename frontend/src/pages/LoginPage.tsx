import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { SplitCard } from "../components/layout/SplitCard"
import { Button } from "../components/ui/Button"
import { Field } from "../components/ui/Field"
import { useSession } from "../context/SessionContext"

export default function LoginPage() {
  const navigate = useNavigate()
  const { signIn } = useSession()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    // TODO: autenticar contra el back cuando exista el endpoint de login.
    signIn(email.split("@")[0])
    navigate("/salas")
  }

  return (
    <SplitCard
      title="Presentá y compartí en tiempo real"
      text="Creá una sala, subí tu archivo en PDF o PowerPoint y mostralo a tu clase en vivo."
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <h2 className="text-xl font-semibold text-brand-700">Iniciar sesión</h2>
        <Field label="Correo electrónico" type="email" autoComplete="email"
          placeholder="nombre@institucion.edu.ar" value={email}
          onChange={(event) => setEmail(event.target.value)} required />
        <Field label="Contraseña" type="password" autoComplete="current-password"
          placeholder="Ingresá tu contraseña" value={password}
          onChange={(event) => setPassword(event.target.value)} required />
        <Button type="submit">Iniciar sesión</Button>
        <p className="text-sm text-ink/70">
          ¿No tenés cuenta?{" "}
          <Link to="/registro" className="font-semibold text-brand-500 hover:underline">Registrate</Link>
          {" · "}
          <Link to="/unirse" className="font-semibold text-brand-500 hover:underline">Entrar como invitado</Link>
        </p>
      </form>
    </SplitCard>
  )
}
