package domain_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"spd.web/services/rooms/internal/dbtest"
	"spd.web/services/rooms/internal/domain"
	"spd.web/services/rooms/internal/store"
)

// Tests de los casos de uso con el Store real: necesitan PostgreSQL (ver dbtest.Open). Cada uno
// trabaja en un esquema propio y vacío. Lo que solo se ve en la base de datos (filas marcadas,
// claves foráneas, bloqueos) se prueba en el paquete store.

const (
	pres5   = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11" // 5 diapositivas en el catálogo falso
	pres3   = "0b8e4c3e-7f4a-4f0e-8d55-2f8f0f6a9b22" // 3 diapositivas
	unknown = "11111111-2222-4333-8444-555555555555" // no existe en el servicio presentations
)

// fakeCatalog simula el servicio presentations: id → número de diapositivas.
type fakeCatalog struct {
	slides map[string]int
	err    error // si no es nil, el servicio no responde
}

func (c fakeCatalog) SlideCount(_ context.Context, id string) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, ok := c.slides[id]
	if !ok {
		return 0, domain.ErrPresentationUnknown
	}
	return n, nil
}

var (
	ctx       = context.Background()
	catalogOK = fakeCatalog{slides: map[string]int{pres5: 5, pres3: 3}}
)

func newService(t *testing.T, catalog domain.Catalog) *domain.Service {
	t.Helper()
	return domain.NewService(store.New(dbtest.Open(t)), catalog)
}

func ptr[T any](v T) *T { return &v }

func mustUser(t *testing.T, svc *domain.Service, email string) domain.User {
	t.Helper()
	u, err := svc.CreateUser(ctx, domain.NewUser{FullName: "Usuario " + email, Email: email, Password: "secreta123"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustRoom(t *testing.T, svc *domain.Service, in domain.NewRoom) domain.Room {
	t.Helper()
	r, err := svc.CreateRoom(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustGroup(t *testing.T, svc *domain.Service, roomID uint, limit, priority int) domain.Group {
	t.Helper()
	g, err := svc.CreateGroup(ctx, roomID, domain.GroupFields{Limit: &limit, Priority: &priority})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func mustAdd(t *testing.T, svc *domain.Service, groupID uint, in domain.NewPresentation) domain.Presentation {
	t.Helper()
	p, err := svc.AddPresentation(ctx, groupID, in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func expect(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err = %v, se esperaba %v", what, err, want)
	}
}

func TestUserLifecycle(t *testing.T) {
	svc := newService(t, catalogOK)

	u, err := svc.CreateUser(ctx, domain.NewUser{FullName: " Ana López ", Email: "Ana@Example.com", Password: "secreta123",
		Roles: []string{"admin", "admin ", "presentador"}})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 || u.FullName != "Ana López" || u.Email != "ana@example.com" || !slices.Equal(u.Roles, []string{"admin", "presentador"}) {
		t.Fatalf("usuario = %+v", u)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("secreta123")) != nil {
		t.Fatal("la contraseña debe guardarse como hash bcrypt")
	}
	data, _ := json.Marshal(u)
	if bytes.Contains(data, []byte("password")) || bytes.Contains(data, []byte("$2a$")) {
		t.Fatalf("el JSON expone la contraseña: %s", data)
	}

	_, err = svc.CreateUser(ctx, domain.NewUser{FullName: "Otra Ana", Email: "ANA@example.com", Password: "secreta123"})
	expect(t, "correo repetido (otras mayúsculas)", err, domain.ErrEmailTaken)
	_, err = svc.CreateUser(ctx, domain.NewUser{FullName: "Sin clave", Email: "x@example.com", Password: "corta"})
	expect(t, "contraseña corta", err, domain.ErrInvalid)

	upd, err := svc.UpdateUser(ctx, u.ID, domain.UserChanges{FullName: ptr("Ana María López"), Roles: &[]string{}, Password: ptr("otra-clave-123")})
	if err != nil {
		t.Fatal(err)
	}
	if upd.FullName != "Ana María López" || len(upd.Roles) != 0 || upd.Email != u.Email || !upd.UpdatedAt.After(u.UpdatedAt) {
		t.Fatalf("modificado = %+v", upd)
	}
	got, err := svc.GetUser(ctx, u.ID)
	if err != nil || got.FullName != upd.FullName || len(got.Roles) != 0 ||
		bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("otra-clave-123")) != nil {
		t.Fatalf("GetUser tras modificar = %+v, %v", got, err)
	}

	beto := mustUser(t, svc, "beto@example.com")
	_, err = svc.UpdateUser(ctx, beto.ID, domain.UserChanges{Email: ptr("ana@example.com")})
	expect(t, "cambiar a un correo ocupado", err, domain.ErrEmailTaken)

	users, err := svc.ListUsers(ctx)
	if err != nil || len(users) != 2 || users[0].ID != u.ID || users[1].ID != beto.ID {
		t.Fatalf("ListUsers = %+v, %v", users, err)
	}

	if err := svc.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.GetUser(ctx, u.ID)
	expect(t, "GetUser tras borrar", err, domain.ErrUserNotFound)
	expect(t, "borrar dos veces", svc.DeleteUser(ctx, u.ID), domain.ErrUserNotFound)
	_, err = svc.UpdateUser(ctx, u.ID, domain.UserChanges{FullName: ptr("x")})
	expect(t, "modificar un borrado", err, domain.ErrUserNotFound)

	// El correo de un usuario borrado vuelve a estar libre.
	again, err := svc.CreateUser(ctx, domain.NewUser{FullName: "Ana", Email: "ana@example.com", Password: "secreta123"})
	if err != nil || again.ID == u.ID {
		t.Fatalf("reutilizar el correo de un usuario borrado: %+v, %v", again, err)
	}
}

func TestRoomAccessKeyAndMembers(t *testing.T) {
	svc := newService(t, catalogOK)
	ana := mustUser(t, svc, "ana@example.com")
	beto := mustUser(t, svc, "beto@example.com")

	locked := mustRoom(t, svc, domain.NewRoom{Name: "Clase 1", URL: "https://spd.example/salas/clase-1", AccessKey: "1234"})
	open := mustRoom(t, svc, domain.NewRoom{Name: "Abierta"})
	data, _ := json.Marshal(locked)
	if !bytes.Contains(data, []byte(`"has_access_key":true`)) || bytes.Contains(data, []byte("$2a$")) || bytes.Contains(data, []byte("1234")) {
		t.Fatalf("JSON de la sala = %s", data)
	}

	for _, key := range []string{"", "4321"} {
		_, _, err := svc.JoinRoom(ctx, locked.ID, domain.Join{UserID: ana.ID, AccessKey: key})
		expect(t, "clave "+key, err, domain.ErrWrongAccessKey)
	}
	m, created, err := svc.JoinRoom(ctx, locked.ID, domain.Join{UserID: ana.ID, AccessKey: "1234"})
	if err != nil || !created || m.UserID != ana.ID || m.RoomID != locked.ID || m.JoinedAt.IsZero() {
		t.Fatalf("unirse con la clave = %+v, %v, %v", m, created, err)
	}
	if _, created, err := svc.JoinRoom(ctx, locked.ID, domain.Join{UserID: ana.ID, AccessKey: "1234"}); err != nil || created {
		t.Fatalf("unirse siendo ya miembro: created = %v, %v", created, err)
	}
	for _, u := range []domain.User{ana, beto} {
		if _, created, err := svc.JoinRoom(ctx, open.ID, domain.Join{UserID: u.ID}); err != nil || !created {
			t.Fatalf("unirse a una sala sin clave: %v, %v", created, err)
		}
	}
	_, _, err = svc.JoinRoom(ctx, open.ID, domain.Join{UserID: 9999})
	expect(t, "usuario inexistente", err, domain.ErrUserNotFound)
	_, _, err = svc.JoinRoom(ctx, 9999, domain.Join{UserID: ana.ID})
	expect(t, "sala inexistente", err, domain.ErrRoomNotFound)
	_, _, err = svc.JoinRoom(ctx, open.ID, domain.Join{})
	expect(t, "sin user_id", err, domain.ErrInvalid)

	if members, err := svc.RoomMembers(ctx, open.ID); err != nil || len(members) != 2 || members[0].ID != ana.ID || members[1].ID != beto.ID {
		t.Fatalf("RoomMembers = %+v, %v", members, err)
	}
	if rooms, err := svc.UserRooms(ctx, ana.ID); err != nil || len(rooms) != 2 || rooms[0].ID != locked.ID || rooms[1].ID != open.ID {
		t.Fatalf("UserRooms = %+v, %v", rooms, err)
	}

	if err := svc.LeaveRoom(ctx, open.ID, ana.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, "salir dos veces", svc.LeaveRoom(ctx, open.ID, ana.ID), domain.ErrNotRoomMember)
	if members, _ := svc.RoomMembers(ctx, open.ID); len(members) != 1 || members[0].ID != beto.ID {
		t.Fatalf("miembros tras salir = %+v", members)
	}
	if _, created, err := svc.JoinRoom(ctx, open.ID, domain.Join{UserID: ana.ID}); err != nil || !created {
		t.Fatalf("volver a la sala: created = %v, %v", created, err)
	}

	// Cambiar la clave y después quitarla.
	if _, err := svc.UpdateRoom(ctx, locked.ID, domain.RoomChanges{AccessKey: ptr("nueva")}); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.JoinRoom(ctx, locked.ID, domain.Join{UserID: beto.ID, AccessKey: "1234"})
	expect(t, "clave antigua", err, domain.ErrWrongAccessKey)
	if _, _, err := svc.JoinRoom(ctx, locked.ID, domain.Join{UserID: beto.ID, AccessKey: "nueva"}); err != nil {
		t.Fatal(err)
	}
	upd, err := svc.UpdateRoom(ctx, locked.ID, domain.RoomChanges{AccessKey: ptr(""), Name: ptr("Clase 1 (abierta)")})
	if err != nil || upd.HasAccessKey() || upd.Name != "Clase 1 (abierta)" || upd.URL != locked.URL {
		t.Fatalf("quitar la clave = %+v, %v", upd, err)
	}

	// Borrar un usuario lo saca de sus salas.
	if err := svc.DeleteUser(ctx, beto.ID); err != nil {
		t.Fatal(err)
	}
	if members, _ := svc.RoomMembers(ctx, open.ID); len(members) != 1 || members[0].ID != ana.ID {
		t.Fatalf("miembros tras borrar a beto = %+v", members)
	}

	for name, in := range map[string]domain.NewRoom{
		"sin nombre":   {Name: " "},
		"URL relativa": {Name: "x", URL: "salas/1"},
		"clave corta":  {Name: "x", AccessKey: "12"},
	} {
		_, err := svc.CreateRoom(ctx, in)
		expect(t, name, err, domain.ErrInvalid)
	}
}

func TestGroupsAndPresentations(t *testing.T) {
	svc := newService(t, catalogOK)
	r := mustRoom(t, svc, domain.NewRoom{Name: "Clase"})

	for name, in := range map[string]domain.GroupFields{
		"sin limit":        {Priority: ptr(1)},
		"sin priority":     {Limit: ptr(5)},
		"cupo cero":        {Limit: ptr(0), Priority: ptr(1)},
		"prioridad cero":   {Limit: ptr(5), Priority: ptr(0)},
		"prioridad menor":  {Limit: ptr(5), Priority: ptr(-1)},
		"cupo negativo":    {Limit: ptr(-1), Priority: ptr(1)},
		"todo por omisión": {},
	} {
		_, err := svc.CreateGroup(ctx, r.ID, in)
		expect(t, name, err, domain.ErrInvalid)
	}
	_, err := svc.CreateGroup(ctx, 9999, domain.GroupFields{Limit: ptr(5), Priority: ptr(1)})
	expect(t, "sala inexistente", err, domain.ErrRoomNotFound)

	// Los grupos se listan en el orden en que pasan a presentar: prioridad 1 primero y, a igual
	// prioridad, el que se creó antes.
	third := mustGroup(t, svc, r.ID, 10, 3)
	first := mustGroup(t, svc, r.ID, 5, 1)
	tie := mustGroup(t, svc, r.ID, 8, 3)
	order := func() []uint {
		groups, err := svc.RoomGroups(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]uint, len(groups))
		for i, g := range groups {
			ids[i] = g.ID
		}
		return ids
	}
	if got := order(); !slices.Equal(got, []uint{first.ID, third.ID, tie.ID}) {
		t.Fatalf("orden de paso = %v", got)
	}
	upd, err := svc.UpdateGroup(ctx, tie.ID, domain.GroupFields{Priority: ptr(2)})
	if err != nil || upd.Priority != 2 || upd.Limit != 8 {
		t.Fatalf("UpdateGroup = %+v, %v", upd, err)
	}
	if got := order(); !slices.Equal(got, []uint{first.ID, tie.ID, third.ID}) {
		t.Fatalf("orden de paso tras cambiar la prioridad = %v", got)
	}
	_, err = svc.UpdateGroup(ctx, tie.ID, domain.GroupFields{Limit: ptr(0)})
	expect(t, "dejar el cupo en cero", err, domain.ErrInvalid)

	// Sin slides se asignan todas las diapositivas; el UUID se normaliza.
	g1, g2 := first, third
	p := mustAdd(t, svc, g1.ID, domain.NewPresentation{PresentationID: strings.ToUpper(pres5)})
	if p.ID != pres5 || p.GroupID != g1.ID || !slices.Equal(p.Slides, []int32{1, 2, 3, 4, 5}) {
		t.Fatalf("presentación = %+v", p)
	}
	_, err = svc.AddPresentation(ctx, g2.ID, domain.NewPresentation{PresentationID: pres5})
	expect(t, "asignar a dos grupos", err, domain.ErrPresentationAssigned)

	p3 := mustAdd(t, svc, g1.ID, domain.NewPresentation{PresentationID: pres3, Slides: &[]int32{3, 1}})
	if !slices.Equal(p3.Slides, []int32{3, 1}) {
		t.Fatalf("diapositivas elegidas = %v", p3.Slides)
	}
	for name, slides := range map[string][]int32{"fuera de rango": {4}, "cero": {0}, "repetida": {1, 1}} {
		_, err := svc.UpdatePresentation(ctx, g1.ID, pres3, domain.PresentationChanges{Slides: &slides})
		expect(t, name, err, domain.ErrInvalid)
	}
	_, err = svc.AddPresentation(ctx, g1.ID, domain.NewPresentation{PresentationID: unknown})
	expect(t, "presentación inexistente", err, domain.ErrPresentationUnknown)
	_, err = svc.AddPresentation(ctx, g1.ID, domain.NewPresentation{PresentationID: "no-es-un-uuid"})
	expect(t, "id que no es UUID", err, domain.ErrInvalid)
	_, err = svc.AddPresentation(ctx, 9999, domain.NewPresentation{PresentationID: unknown})
	expect(t, "grupo inexistente (antes que la presentación)", err, domain.ErrGroupNotFound)

	if list, err := svc.GroupPresentations(ctx, g1.ID); err != nil || len(list) != 2 || list[0].ID != pres5 || list[1].ID != pres3 {
		t.Fatalf("GroupPresentations = %+v, %v", list, err)
	}
	_, err = svc.GetPresentation(ctx, g2.ID, pres3)
	expect(t, "presentación de otro grupo", err, domain.ErrPresentationNotFound)

	empty, err := svc.UpdatePresentation(ctx, g1.ID, pres5, domain.PresentationChanges{Slides: &[]int32{}})
	if data, _ := json.Marshal(empty); err != nil || !bytes.Contains(data, []byte(`"slides":[]`)) {
		t.Fatalf("sin diapositivas: %s, %v", data, err)
	}

	// Quitada de un grupo, la presentación se puede asignar a otro.
	if err := svc.RemovePresentation(ctx, g1.ID, pres5); err != nil {
		t.Fatal(err)
	}
	expect(t, "quitar dos veces", svc.RemovePresentation(ctx, g1.ID, pres5), domain.ErrPresentationNotFound)
	if moved := mustAdd(t, svc, g2.ID, domain.NewPresentation{PresentationID: pres5, Slides: &[]int32{2}}); moved.GroupID != g2.ID {
		t.Fatalf("reasignada = %+v", moved)
	}

	// Borrar un grupo borra sus presentaciones.
	if err := svc.DeleteGroup(ctx, g1.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.GetGroup(ctx, g1.ID)
	expect(t, "grupo borrado", err, domain.ErrGroupNotFound)
	_, err = svc.GroupPresentations(ctx, g1.ID)
	expect(t, "presentaciones de un grupo borrado", err, domain.ErrGroupNotFound)
	mustAdd(t, svc, g2.ID, domain.NewPresentation{PresentationID: pres3}) // pres3 quedó libre
}

// DetalleGrupo: usuarios y grupos se relacionan n:m, siempre dentro de la sala y del cupo.
func TestGroupMembers(t *testing.T) {
	svc := newService(t, catalogOK)
	ana := mustUser(t, svc, "ana@example.com")
	beto := mustUser(t, svc, "beto@example.com")
	caro := mustUser(t, svc, "caro@example.com")
	r := mustRoom(t, svc, domain.NewRoom{Name: "Clase"})
	other := mustRoom(t, svc, domain.NewRoom{Name: "Otra clase"})
	g := mustGroup(t, svc, r.ID, 2, 1)
	g2 := mustGroup(t, svc, r.ID, 5, 2)
	gOther := mustGroup(t, svc, other.ID, 5, 1)
	join := func(groupID uint, u domain.User) (domain.GroupMember, bool, error) {
		return svc.JoinGroup(ctx, groupID, domain.GroupJoin{UserID: u.ID})
	}

	// Para entrar a un grupo hay que ser miembro de su sala: si no, la clave de ingreso de la
	// sala se podría saltar entrando por el grupo.
	_, _, err := join(g.ID, ana)
	expect(t, "sin ser miembro de la sala", err, domain.ErrRoomMembershipRequired)
	for _, u := range []domain.User{ana, beto, caro} {
		if _, _, err := svc.JoinRoom(ctx, r.ID, domain.Join{UserID: u.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := svc.JoinRoom(ctx, other.ID, domain.Join{UserID: ana.ID}); err != nil {
		t.Fatal(err)
	}

	m, created, err := join(g.ID, ana)
	if err != nil || !created || m.UserID != ana.ID || m.GroupID != g.ID || m.JoinedAt.IsZero() {
		t.Fatalf("entrar al grupo = %+v, %v, %v", m, created, err)
	}
	if _, created, err := join(g.ID, ana); err != nil || created {
		t.Fatalf("entrar siendo ya integrante: created = %v, %v", created, err)
	}
	if _, _, err := join(g.ID, beto); err != nil {
		t.Fatal(err)
	}
	_, _, err = join(g.ID, caro)
	expect(t, "grupo completo (cupo 2)", err, domain.ErrGroupFull)
	if _, created, err := join(g.ID, ana); err != nil || created {
		t.Fatalf("un integrante puede repetir el alta aunque el grupo esté lleno: %v, %v", created, err)
	}
	_, _, err = join(9999, ana)
	expect(t, "grupo inexistente", err, domain.ErrGroupNotFound)
	_, _, err = svc.JoinGroup(ctx, g.ID, domain.GroupJoin{UserID: 9999})
	expect(t, "usuario inexistente", err, domain.ErrUserNotFound)
	_, _, err = svc.JoinGroup(ctx, g.ID, domain.GroupJoin{})
	expect(t, "sin user_id", err, domain.ErrInvalid)

	// n:m: ana está en dos grupos de la sala y en uno de otra sala.
	for _, id := range []uint{g2.ID, gOther.ID} {
		if _, _, err := join(id, ana); err != nil {
			t.Fatal(err)
		}
	}
	groupIDs := func(u domain.User) []uint {
		groups, err := svc.UserGroups(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]uint, len(groups))
		for i, gr := range groups {
			ids[i] = gr.ID
		}
		return ids
	}
	if got := groupIDs(ana); !slices.Equal(got, []uint{g.ID, g2.ID, gOther.ID}) {
		t.Fatalf("grupos de ana = %v", got)
	}
	if members, err := svc.GroupMembers(ctx, g.ID); err != nil || len(members) != 2 || members[0].ID != ana.ID || members[1].ID != beto.ID {
		t.Fatalf("GroupMembers = %+v, %v", members, err)
	}

	// El cupo no puede quedar por debajo de los integrantes actuales.
	_, err = svc.UpdateGroup(ctx, g.ID, domain.GroupFields{Limit: ptr(1)})
	expect(t, "cupo menor que los integrantes", err, domain.ErrLimitBelowMembers)
	if upd, err := svc.UpdateGroup(ctx, g.ID, domain.GroupFields{Limit: ptr(3)}); err != nil || upd.Limit != 3 {
		t.Fatalf("ampliar el cupo = %+v, %v", upd, err)
	}
	if _, _, err := join(g.ID, caro); err != nil {
		t.Fatalf("con el cupo ampliado, caro debería entrar: %v", err)
	}

	// Salir de un grupo libera la plaza; salir de la sala saca de todos sus grupos.
	if err := svc.LeaveGroup(ctx, g.ID, caro.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, "salir dos veces", svc.LeaveGroup(ctx, g.ID, caro.ID), domain.ErrNotGroupMember)
	if err := svc.LeaveRoom(ctx, r.ID, ana.ID); err != nil {
		t.Fatal(err)
	}
	if got := groupIDs(ana); !slices.Equal(got, []uint{gOther.ID}) {
		t.Fatalf("tras salir de la sala, ana solo debería seguir en el grupo de la otra sala: %v", got)
	}

	// Borrar al usuario, o el grupo, termina sus pertenencias.
	if err := svc.DeleteUser(ctx, beto.ID); err != nil {
		t.Fatal(err)
	}
	if members, _ := svc.GroupMembers(ctx, g.ID); len(members) != 0 {
		t.Fatalf("integrantes tras borrar a beto = %+v", members)
	}
	if err := svc.DeleteGroup(ctx, gOther.ID); err != nil {
		t.Fatal(err)
	}
	if got := groupIDs(ana); len(got) != 0 {
		t.Fatalf("tras borrar el grupo, ana no debería estar en ninguno: %v", got)
	}
	_, err = svc.GroupMembers(ctx, gOther.ID)
	expect(t, "integrantes de un grupo borrado", err, domain.ErrGroupNotFound)
}

func TestCatalogUnavailable(t *testing.T) {
	svc := newService(t, fakeCatalog{err: errors.New("connection refused")})
	r := mustRoom(t, svc, domain.NewRoom{Name: "Clase"})
	g := mustGroup(t, svc, r.ID, 1, 1)
	_, err := svc.AddPresentation(ctx, g.ID, domain.NewPresentation{PresentationID: pres5})
	expect(t, "presentations caído", err, domain.ErrCatalogUnavailable)
}

func TestDeleteRoomCascades(t *testing.T) {
	svc := newService(t, catalogOK)
	ana := mustUser(t, svc, "ana@example.com")
	r := mustRoom(t, svc, domain.NewRoom{Name: "Se borra"})
	other := mustRoom(t, svc, domain.NewRoom{Name: "Se queda"})
	for _, id := range []uint{r.ID, other.ID} {
		if _, _, err := svc.JoinRoom(ctx, id, domain.Join{UserID: ana.ID}); err != nil {
			t.Fatal(err)
		}
	}
	g := mustGroup(t, svc, r.ID, 1, 1)
	kept := mustGroup(t, svc, other.ID, 1, 1)
	mustAdd(t, svc, g.ID, domain.NewPresentation{PresentationID: pres5})
	mustAdd(t, svc, kept.ID, domain.NewPresentation{PresentationID: pres3})

	if err := svc.DeleteRoom(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	_, err := svc.GetRoom(ctx, r.ID)
	expect(t, "sala borrada", err, domain.ErrRoomNotFound)
	expect(t, "borrar dos veces", svc.DeleteRoom(ctx, r.ID), domain.ErrRoomNotFound)
	_, err = svc.GetGroup(ctx, g.ID)
	expect(t, "grupo de la sala", err, domain.ErrGroupNotFound)
	_, err = svc.GetPresentation(ctx, g.ID, pres5)
	expect(t, "presentación de la sala", err, domain.ErrPresentationNotFound)
	if rooms, _ := svc.UserRooms(ctx, ana.ID); len(rooms) != 1 || rooms[0].ID != other.ID {
		t.Fatalf("salas de ana = %+v", rooms)
	}
	if _, err := svc.GetPresentation(ctx, kept.ID, pres3); err != nil {
		t.Fatalf("la otra sala no debe verse afectada: %v", err)
	}

	// Una sala borrada no admite hijos nuevos, y su presentación quedó libre.
	_, err = svc.CreateGroup(ctx, r.ID, domain.GroupFields{Limit: ptr(1), Priority: ptr(1)})
	expect(t, "grupo en sala borrada", err, domain.ErrRoomNotFound)
	_, _, err = svc.JoinRoom(ctx, r.ID, domain.Join{UserID: ana.ID})
	expect(t, "unirse a sala borrada", err, domain.ErrRoomNotFound)
	mustAdd(t, svc, kept.ID, domain.NewPresentation{PresentationID: pres5})
}
