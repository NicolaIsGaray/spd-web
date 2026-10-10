// Package store guarda las entidades del dominio en PostgreSQL con GORM: modelos y migración,
// consultas, transacciones y borrado lógico (soft delete) en cascada. Implementa domain.Store.
package store

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spd.web/services/rooms/internal/domain"
)

var _ domain.Store = (*Store)(nil)

// Store implementa domain.Store. Es seguro para uso concurrente.
//
// El borrado lógico no dispara las claves foráneas (la fila sigue existiendo), así que Store
// borra en cascada él mismo, en una transacción. Para que un alta concurrente no deje un hijo
// huérfano, cada alta bloquea a su padre con FOR SHARE (ver lockShare).
type Store struct {
	db *gorm.DB
}

// New crea el Store sobre una conexión abierta con Open.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

var (
	// lockShare bloquea la fila padre hasta el final de la transacción en la que se le añade un
	// hijo. Un borrado concurrente del padre espera y después borra también al hijo nuevo; si el
	// borrado llegó antes, el alta espera y ya no encuentra al padre.
	lockShare = clause.Locking{Strength: "SHARE"}
	// lockUpdate bloquea la fila que se va a modificar.
	lockUpdate = clause.Locking{Strength: "UPDATE"}
)

// take carga en dest la fila (no borrada) que cumple la condición, o devuelve notFound.
func take(tx *gorm.DB, dest any, notFound error, query string, args ...any) error {
	err := tx.Where(query, args...).Take(dest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return err
}

// deleted traduce el resultado de un borrado lógico: sin filas afectadas, no existía.
func deleted(res *gorm.DB, notFound error) error {
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return notFound
	}
	return nil
}

// update bloquea la fila id, le aplica changes (columna → valor) y deja en dest la fila
// actualizada.
func (s *Store) update(ctx context.Context, dest any, id uint, notFound error, changes map[string]any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := take(tx.Clauses(lockUpdate), dest, notFound, "id = ?", id); err != nil {
			return err
		}
		if len(changes) == 0 {
			return nil
		}
		return tx.Model(dest).Updates(changes).Error // también actualiza dest
	})
}

// reactivate es el ON CONFLICT de las tablas con clave natural (room_members, group_members y
// presentations), que no pueden repetir la clave de una fila borrada: si la fila existe y está
// borrada, se reactiva con los valores nuevos de columns; si está activa no se toca y el alta no
// afecta a ninguna fila.
func reactivate(table string, key []string, columns ...string) clause.OnConflict {
	cols := make([]clause.Column, len(key))
	for i, k := range key {
		cols[i] = clause.Column{Name: k}
	}
	return clause.OnConflict{
		Columns:   cols,
		DoUpdates: clause.AssignmentColumns(append(columns, "deleted_at")),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: table + ".deleted_at IS NOT NULL"}}},
	}
}
