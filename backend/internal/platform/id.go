package platform

import "github.com/google/uuid"

// CanonicalUUID valida el identificador de una presentación y lo devuelve en forma canónica
// (minúsculas, con guiones). Usar siempre esa forma evita que "ABC…" y "abc…" acaben en
// sesiones o directorios distintos, y garantiza que el id es seguro como nombre de directorio.
func CanonicalUUID(s string) (string, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return "", false
	}
	return id.String(), true
}
