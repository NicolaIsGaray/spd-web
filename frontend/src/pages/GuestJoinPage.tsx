import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router-dom"
import { SplitCard } from "../components/layout/SplitCard"
import { Button } from "../components/ui/Button"
import { Field } from "../components/ui/Field"
import { useSession } from "../context/SessionContext"

export default function GuestJoinPage() {
  const navigate = useNavigate()
  const { signIn } = useSession()
  const [name, setName] = useState("")
  const [key, setKey] = useState("")

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    // TODO: validar la clave contra el back y unirse a la sala real.
    signIn(name)
    navigate("/salas/4")
  }

  return (
    <SplitCard title="Todo listo para entrar en sala" text="Ingresá tus datos y unite a la presentación en vivo.">
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <h2 className="text-xl font-semibold text-brand-700">Unite a la sala</h2>
        <Field label="Nombre de usuario" placeholder="¿Cómo te llamás?" value={name}
          onChange={(event) => setName(event.target.value)} required />
        <Field label="Clave de la sala" placeholder="Ej. A7K-29P" value={key}
          onChange={(event) => setKey(event.target.value.toUpperCase())} required />
        <Button type="submit">Ingresar</Button>
      </form>
    </SplitCard>
  )
}
