package store

import (
	"database/sql/driver"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Array guarda un slice de Go en una columna array de PostgreSQL (text[], integer[]...). GORM
// trabaja sobre database/sql, que no sabe leer ni escribir arrays, así que el valor se convierte
// a la representación textual de PostgreSQL ({a,"b c"}) con pgtype, el sistema de tipos de pgx.
//
// pgtype.Map no es seguro para uso concurrente, así que se crea uno por conversión: es barato,
// porque delega en el mapa de tipos por defecto.
type Array[T any] []T

// Scan implementa sql.Scanner. Un NULL se lee como un array vacío.
func (a *Array[T]) Scan(src any) error {
	var v []T
	if err := pgtype.NewMap().SQLScanner(&v).Scan(src); err != nil {
		return err
	}
	if v == nil {
		v = []T{}
	}
	*a = v
	return nil
}

// Value implementa driver.Valuer. Un slice nil se guarda como un array vacío, nunca como NULL.
func (a Array[T]) Value() (driver.Value, error) {
	v := []T(a)
	if v == nil {
		v = []T{}
	}
	m := pgtype.NewMap()
	t, ok := m.TypeForValue(v)
	if !ok {
		return nil, fmt.Errorf("PostgreSQL no tiene un tipo array para %T", v)
	}
	buf, err := m.Encode(t.OID, pgtype.TextFormatCode, v, nil)
	if err != nil {
		return nil, err
	}
	return string(buf), nil
}
