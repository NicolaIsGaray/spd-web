// "2026-06-21T14:03:00Z" → "21/06/2026". Si la fecha no se entiende, devuelve el texto original.
export function formatDate(iso: string) {
  const date = new Date(iso)
  if (!Number.isFinite(date.getTime())) return iso
  return new Intl.DateTimeFormat("es-AR", { day: "2-digit", month: "2-digit", year: "numeric" }).format(date)
}
