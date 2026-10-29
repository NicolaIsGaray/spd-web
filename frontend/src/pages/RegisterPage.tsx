import { useState, type FormEvent } from "react"
import { Link, useNavigate } from "react-router-dom"
import { SplitCard } from "../components/layout/SplitCard"
import { Button } from "../components/ui/Button"
import { Field } from "../components/ui/Field"
import { useSession } from "../context/SessionContext"
import { createUser } from "../services/rooms"

export default function RegisterPage() {
  const navigate = useNavigate()
  const { signIn } = useSession()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [error, setError] = useState("")
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (password !== confirm) return setError("Las contraseñas no coinciden.")
    if (password.length < 8) return setError("La contraseña debe tener al menos 8 caracteres.")
    setSubmitting(true)
    setError("")
    // POST /api/users: la contraseña queda hasheada en el back. Sin endpoint de login todavía,
    // la sesión arranca recién creada (el userId sirve para unirse a salas y grupos).
    createUser({ fullName: name, email, password })
      .then((user) => {
        signIn({ name: user.full_name, userId: user.id })
        navigate("/salas")
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "No se pudo crear la cuenta.")
        setSubmitting(false)
      })
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
        <Button type="submit" disabled={submitting}>{submitting ? "Creando cuenta…" : "Crear cuenta"}</Button>
        <p className="text-sm text-ink/70">
          ¿Ya tenés cuenta?{" "}
          <Link to="/login" className="font-semibold text-brand-500 hover:underline">Iniciá sesión</Link>
        </p>
      </form>
    </SplitCard>
  )
}
