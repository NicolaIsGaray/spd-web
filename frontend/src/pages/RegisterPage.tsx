import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { SplitCard } from "../components/layout/SplitCard"
import { Button } from "../components/ui/Button"
import { Field } from "../components/ui/Field"
import { useSession } from "../context/SessionContext"

export default function RegisterPage() {
  const navigate = useNavigate()
  const { signIn } = useSession()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [error, setError] = useState("")

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (password !== confirm) return setError("Las contraseñas no coinciden.")
    // TODO: crear el usuario en el back (rooms) y luego iniciar sesión.
    signIn(name)
    navigate("/salas")
  }

  return (
    <SplitCard title="Creá tu cuenta" text="Registrate para crear salas y presentar a tu clase en vivo.">
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <h2 className="text-xl font-semibold text-brand-700">Registrate</h2>
        <Field label="Nombre y apellido" autoComplete="name" value={name}
          onChange={(event) => setName(event.target.value)} required />
        <Field label="Correo electrónico" type="email" autoComplete="email" value={email}
          onChange={(event) => setEmail(event.target.value)} required />
        <Field label="Contraseña" type="password" autoComplete="new-password" value={password}
          onChange={(event) => setPassword(event.target.value)} required />
        <Field label="Repetir contraseña" type="password" autoComplete="new-password" value={confirm}
          onChange={(event) => { setConfirm(event.target.value); setError("") }} required />
        {error && <p role="alert" className="text-sm font-medium text-red-700">{error}</p>}
        <Button type="submit">Crear cuenta</Button>
        <p className="text-sm text-ink/70">
          ¿Ya tenés cuenta?{" "}
          <Link to="/login" className="font-semibold text-brand-500 hover:underline">Iniciá sesión</Link>
        </p>
      </form>
    </SplitCard>
  )
}
