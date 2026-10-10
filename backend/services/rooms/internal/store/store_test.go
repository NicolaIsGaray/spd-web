package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"

	"spd.web/services/rooms/internal/dbtest"
	"spd.web/services/rooms/internal/domain"
	"spd.web/services/rooms/internal/store"
)

// Tests de persistencia: lo que solo se ve en la base de datos (filas marcadas, claves foráneas,
// reactivación de filas, bloqueos). Necesitan PostgreSQL (ver dbtest.Open). Las reglas de
// negocio se prueban en el paquete domain.

const (
	pres1 = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"
	pres2 = "0b8e4c3e-7f4a-4f0e-8d55-2f8f0f6a9b22"
)

var ctx = context.Background()

func newStore(t *testing.T) (*store.Store, *gorm.DB) {
	t.Helper()
	db := dbtest.Open(t)
	return store.New(db), db
}

// count ejecuta una consulta SELECT count(*) …
func count(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func expect(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err = %v, se esperaba %v", what, err, want)
	}
}

func admitAll(domain.Room) error { return nil }

// fixture crea un usuario miembro de una sala y de uno de sus grupos, con una presentación
// asignada al grupo.
func fixture(t *testing.T, st *store.Store) (domain.User, domain.Room, domain.Group) {
	t.Helper()
	u, err := st.CreateUser(ctx, domain.User{FullName: "Ana", Email: "ana@example.com", PasswordHash: "hash", Roles: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.CreateRoom(ctx, domain.Room{Name: "Clase"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.AddRoomMember(ctx, r.ID, u.ID, admitAll); err != nil {
		t.Fatal(err)
	}
	g, err := st.CreateGroup(ctx, domain.Group{RoomID: r.ID, Limit: 5, Priority: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.AddGroupMember(ctx, g.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddPresentation(ctx, domain.Presentation{ID: pres1, GroupID: g.ID, Slides: []int32{1, 2}}); err != nil {
		t.Fatal(err)
	}
	return u, r, g
}

// newUser crea un usuario con un correo propio.
func newUser(t *testing.T, st *store.Store, email string) domain.User {
	t.Helper()
	u, err := st.CreateUser(ctx, domain.User{FullName: email, Email: email, PasswordHash: "hash", Roles: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// Borrar no elimina filas: las marca con deleted_at, también las de la cascada.
func TestSoftDeleteKeepsTheRows(t *testing.T) {
	st, db := newStore(t)
	u, r, g := fixture(t, st)

	if err := st.DeleteRoom(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	for table, id := range map[string]any{"users": u.ID, "rooms": r.ID, "groups": g.ID, "presentations": pres1} {
		if n := count(t, db, "SELECT count(*) FROM "+table+" WHERE id = ? AND deleted_at IS NOT NULL", id); n != 1 {
			t.Fatalf("%s: filas marcadas = %d, se esperaba 1", table, n)
		}
	}
	if n := count(t, db, "SELECT count(*) FROM room_members WHERE room_id = ? AND deleted_at IS NOT NULL", r.ID); n != 1 {
		t.Fatalf("room_members: filas marcadas = %d, se esperaba 1", n)
	}
	if n := count(t, db, "SELECT count(*) FROM group_members WHERE group_id = ? AND deleted_at IS NOT NULL", g.ID); n != 1 {
		t.Fatalf("group_members: filas marcadas = %d, se esperaba 1", n)
	}
	if n := count(t, db, "SELECT count(*) FROM groups WHERE deleted_at IS NULL"); n != 0 {
		t.Fatalf("quedaron %d grupos activos", n)
	}
}

// Las claves naturales no se pueden repetir: volver a una sala, o reasignar una presentación
// quitada, reactiva la misma fila con los datos nuevos.
func TestRejoinAndReassignReuseTheRow(t *testing.T) {
	st, db := newStore(t)
	u, r, g := fixture(t, st)

	if _, created, err := st.AddRoomMember(ctx, r.ID, u.ID, admitAll); err != nil || created {
		t.Fatalf("unirse siendo miembro: created = %v, %v", created, err)
	}
	if err := st.RemoveRoomMember(ctx, r.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, created, err := st.AddRoomMember(ctx, r.ID, u.ID, admitAll); err != nil || !created {
		t.Fatalf("volver a la sala: created = %v, %v", created, err)
	}
	if n := count(t, db, "SELECT count(*) FROM room_members WHERE room_id = ? AND user_id = ?", r.ID, u.ID); n != 1 {
		t.Fatalf("filas de la pertenencia = %d, se esperaba 1", n)
	}
	// Al salir de la sala salió también del grupo: volver al grupo reactiva su fila.
	if _, created, err := st.AddGroupMember(ctx, g.ID, u.ID); err != nil || !created {
		t.Fatalf("volver al grupo: created = %v, %v", created, err)
	}
	if n := count(t, db, "SELECT count(*) FROM group_members WHERE group_id = ? AND user_id = ?", g.ID, u.ID); n != 1 {
		t.Fatalf("filas del integrante = %d, se esperaba 1", n)
	}

	other, err := st.CreateGroup(ctx, domain.Group{RoomID: r.ID, Limit: 3, Priority: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.AddPresentation(ctx, domain.Presentation{ID: pres1, GroupID: other.ID, Slides: []int32{1}})
	expect(t, "asignada en otro grupo", err, domain.ErrPresentationAssigned)
	if err := st.RemovePresentation(ctx, g.ID, pres1); err != nil {
		t.Fatal(err)
	}
	p, err := st.AddPresentation(ctx, domain.Presentation{ID: pres1, GroupID: other.ID, Slides: []int32{3}})
	if err != nil || p.GroupID != other.ID || len(p.Slides) != 1 || p.Slides[0] != 3 {
		t.Fatalf("reasignar = %+v, %v", p, err)
	}
	if n := count(t, db, "SELECT count(*) FROM presentations WHERE id = ?", pres1); n != 1 {
		t.Fatalf("filas de la presentación = %d, se esperaba 1", n)
	}
}

// Si admit rechaza la entrada, no se crea la pertenencia.
func TestAddMemberRejectedByAdmit(t *testing.T) {
	st, db := newStore(t)
	u, r, _ := fixture(t, st)
	if err := st.RemoveRoomMember(ctx, r.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err := st.AddRoomMember(ctx, r.ID, u.ID, func(domain.Room) error { return domain.ErrWrongAccessKey })
	expect(t, "admit rechaza", err, domain.ErrWrongAccessKey)
	if n := count(t, db, "SELECT count(*) FROM room_members WHERE deleted_at IS NULL"); n != 0 {
		t.Fatalf("pertenencias activas = %d, se esperaba 0", n)
	}
}

// El índice único del correo es parcial: solo cubre a los usuarios no borrados.
func TestEmailUniqueAmongActiveUsers(t *testing.T) {
	st, db := newStore(t)
	u, _, _ := fixture(t, st)
	_, err := st.CreateUser(ctx, domain.User{FullName: "Otra", Email: u.Email, PasswordHash: "hash"})
	expect(t, "correo repetido", err, domain.ErrEmailTaken)
	if err := st.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, domain.User{FullName: "Otra", Email: u.Email, PasswordHash: "hash"}); err != nil {
		t.Fatalf("reutilizar el correo de un usuario borrado: %v", err)
	}
	if n := count(t, db, "SELECT count(*) FROM users WHERE email = ?", u.Email); n != 2 {
		t.Fatalf("filas con el correo = %d, se esperaban 2 (una borrada)", n)
	}
}

// AutoMigrate crea las claves foráneas y se puede ejecutar en cada arranque.
func TestSchema(t *testing.T) {
	_, db := newStore(t)
	if err := store.Migrate(db); err != nil {
		t.Fatalf("una segunda migración debe ser inocua: %v", err)
	}
	err := db.Exec(`INSERT INTO groups (room_id, "limit", priority) VALUES (9999, 1, 1)`).Error
	expect(t, "grupo de una sala inexistente", err, gorm.ErrForeignKeyViolated)
	err = db.Exec(`INSERT INTO room_members (user_id, room_id) VALUES (9999, 9999)`).Error
	expect(t, "pertenencia inexistente", err, gorm.ErrForeignKeyViolated)
	err = db.Exec(`INSERT INTO presentations (id, group_id) VALUES (?, 9999)`, pres2).Error
	expect(t, "presentación de un grupo inexistente", err, gorm.ErrForeignKeyViolated)
	err = db.Exec(`INSERT INTO group_members (user_id, group_id) VALUES (9999, 9999)`).Error
	expect(t, "integrante inexistente", err, gorm.ErrForeignKeyViolated)
}

// Salir de una sala, borrar al usuario o borrar el grupo marcan sus filas de DetalleGrupo.
func TestGroupMembershipCascades(t *testing.T) {
	st, db := newStore(t)
	u, r, g := fixture(t, st)
	active := func() int64 {
		return count(t, db, "SELECT count(*) FROM group_members WHERE group_id = ? AND deleted_at IS NULL", g.ID)
	}

	if err := st.RemoveRoomMember(ctx, r.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if n := active(); n != 0 {
		t.Fatalf("tras salir de la sala quedan %d integrantes activos", n)
	}
	if _, _, err := st.AddRoomMember(ctx, r.ID, u.ID, admitAll); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.AddGroupMember(ctx, g.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if n := active(); n != 0 {
		t.Fatalf("tras borrar el grupo quedan %d integrantes activos", n)
	}
	if n := count(t, db, "SELECT count(*) FROM group_members WHERE group_id = ?", g.ID); n != 1 {
		t.Fatalf("la fila del integrante debe conservarse marcada: hay %d", n)
	}
}

// Altas concurrentes en grupos con cupo: gracias al bloqueo FOR UPDATE del grupo, en cada uno
// entran exactamente tantos como el cupo y el resto recibe ErrGroupFull.
func TestGroupCupoUnderConcurrency(t *testing.T) {
	st, db := newStore(t)
	r, err := st.CreateRoom(ctx, domain.Room{Name: "Clase"})
	if err != nil {
		t.Fatal(err)
	}
	const cupo, candidates, rounds = 3, 10, 5
	users := make([]domain.User, candidates)
	for i := range users {
		users[i] = newUser(t, st, fmt.Sprintf("u%d@example.com", i))
		if _, _, err := st.AddRoomMember(ctx, r.ID, users[i].ID, admitAll); err != nil {
			t.Fatal(err)
		}
	}

	for range rounds {
		g, err := st.CreateGroup(ctx, domain.Group{RoomID: r.ID, Limit: cupo, Priority: 1})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		joined, full := 0, 0
		for _, u := range users {
			wg.Go(func() {
				_, _, err := st.AddGroupMember(ctx, g.ID, u.ID)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					joined++
				case errors.Is(err, domain.ErrGroupFull):
					full++
				default:
					t.Errorf("AddGroupMember: %v", err)
				}
			})
		}
		wg.Wait()
		members := count(t, db, "SELECT count(*) FROM group_members WHERE group_id = ? AND deleted_at IS NULL", g.ID)
		if joined != cupo || full != candidates-cupo || members != cupo {
			t.Fatalf("entraron %d (en la base, %d) y quedaron fuera %d; cupo %d", joined, members, full, cupo)
		}
	}
}

// Entrar a un grupo mientras se sale de su sala: gracias al bloqueo FOR SHARE de la pertenencia a
// la sala, nadie queda en un grupo sin ser miembro de su sala.
func TestGroupMembersStayInTheRoom(t *testing.T) {
	st, db := newStore(t)
	r, err := st.CreateRoom(ctx, domain.Room{Name: "Clase"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.CreateGroup(ctx, domain.Group{RoomID: r.ID, Limit: 100, Priority: 1})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		u := newUser(t, st, fmt.Sprintf("u%d@example.com", i))
		if _, _, err := st.AddRoomMember(ctx, r.ID, u.ID, admitAll); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Go(func() {
			_, _, err := st.AddGroupMember(ctx, g.ID, u.ID)
			if err != nil && !errors.Is(err, domain.ErrRoomMembershipRequired) {
				t.Errorf("AddGroupMember: %v", err)
			}
		})
		wg.Go(func() {
			if err := st.RemoveRoomMember(ctx, r.ID, u.ID); err != nil {
				t.Errorf("RemoveRoomMember: %v", err)
			}
		})
		wg.Wait()
	}
	orphans := count(t, db, `SELECT count(*) FROM group_members gm JOIN groups g ON g.id = gm.group_id
		WHERE gm.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM room_members rm
			WHERE rm.user_id = gm.user_id AND rm.room_id = g.room_id AND rm.deleted_at IS NULL)`)
	if orphans != 0 {
		t.Fatalf("integrantes de grupos que ya no son miembros de la sala = %d", orphans)
	}
}

// Altas de grupos concurrentes con el borrado de su sala: gracias al bloqueo FOR SHARE, ningún
// grupo activo puede quedar colgando de una sala borrada.
func TestConcurrentDeleteLeavesNoOrphans(t *testing.T) {
	st, db := newStore(t)
	for range 5 {
		r, err := st.CreateRoom(ctx, domain.Room{Name: "Carrera"})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				_, err := st.CreateGroup(ctx, domain.Group{RoomID: r.ID, Limit: 1, Priority: 1})
				if err != nil && !errors.Is(err, domain.ErrRoomNotFound) {
					t.Errorf("CreateGroup: %v", err)
				}
			})
		}
		wg.Go(func() {
			if err := st.DeleteRoom(ctx, r.ID); err != nil {
				t.Errorf("DeleteRoom: %v", err)
			}
		})
		wg.Wait()
	}
	orphans := count(t, db, `SELECT count(*) FROM groups g JOIN rooms r ON r.id = g.room_id
		WHERE g.deleted_at IS NULL AND r.deleted_at IS NOT NULL`)
	if orphans != 0 {
		t.Fatalf("grupos activos en salas borradas = %d", orphans)
	}
}

// La respuesta de un alta muestra las mismas fechas que una lectura posterior.
func TestTimestampsMatchTheDatabase(t *testing.T) {
	st, _ := newStore(t)
	created, err := st.CreateRoom(ctx, domain.Room{Name: "Clase"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := st.GetRoom(ctx, created.ID)
	if err != nil || !read.CreatedAt.Equal(created.CreatedAt) || !read.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("alta %v / %v, lectura %v / %v (%v)", created.CreatedAt, created.UpdatedAt, read.CreatedAt, read.UpdatedAt, err)
	}
}
