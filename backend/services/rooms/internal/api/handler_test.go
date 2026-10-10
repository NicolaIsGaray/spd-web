package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/rooms/internal/dbtest"
	"spd.web/services/rooms/internal/domain"
	"spd.web/services/rooms/internal/store"
)

// Tests de integración: HTTP → servicio → PostgreSQL (ver dbtest.Open).

const (
	pres    = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11" // 4 diapositivas
	unknown = "11111111-2222-4333-8444-555555555555" // no existe en presentations
	downPID = "22222222-3333-4444-8555-666666666666" // presentations no responde
)

type fakeCatalog struct{}

func (fakeCatalog) SlideCount(_ context.Context, id string) (int, error) {
	switch id {
	case pres:
		return 4, nil
	case downPID:
		return 0, errors.New("dial tcp 10.0.0.7:8081: connection refused")
	}
	return 0, domain.ErrPresentationUnknown
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.DiscardHandler)
	engine := platform.NewEngine(log)
	New(domain.NewService(store.New(dbtest.Open(t)), fakeCatalog{}), log).Register(engine)
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv
}

// call envía la petición (body se codifica como JSON salvo que ya sea un string) y devuelve la
// respuesta con su cuerpo.
func call(t *testing.T, srv *httptest.Server, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, srv.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

// want comprueba el estado y devuelve el cuerpo decodificado como objeto JSON.
func want(t *testing.T, srv *httptest.Server, status int, method, path string, body any) map[string]any {
	t.Helper()
	resp, data := call(t, srv, method, path, body)
	if resp.StatusCode != status {
		t.Fatalf("%s %s = %d %s; se esperaba %d", method, path, resp.StatusCode, data, status)
	}
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func TestUserEndpoints(t *testing.T) {
	srv := newServer(t)
	if _, data := call(t, srv, http.MethodGet, "/api/users", nil); string(data) != "[]" {
		t.Fatalf("lista vacía = %s, se esperaba []", data)
	}

	resp, data := call(t, srv, http.MethodPost, "/api/users", map[string]any{
		"full_name": "Ana López", "email": "ana@example.com", "password": "secreta123", "roles": []string{"admin"}})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("alta = %d %s", resp.StatusCode, data)
	}
	var u map[string]any
	_ = json.Unmarshal(data, &u)
	id := fmt.Sprint(u["id"])
	if resp.Header.Get("Location") != "/api/users/"+id || u["email"] != "ana@example.com" || bytes.Contains(data, []byte("password")) {
		t.Fatalf("alta: Location = %q, cuerpo = %s", resp.Header.Get("Location"), data)
	}

	want(t, srv, http.StatusOK, http.MethodGet, "/api/users/"+id, nil)
	got := want(t, srv, http.StatusOK, http.MethodPatch, "/api/users/"+id, map[string]any{"full_name": "Ana María"})
	if got["full_name"] != "Ana María" || got["email"] != "ana@example.com" {
		t.Fatalf("PATCH = %v", got)
	}
	errBody := want(t, srv, http.StatusConflict, http.MethodPost, "/api/users", map[string]any{
		"full_name": "Otra", "email": "ANA@example.com", "password": "secreta123"})
	if errBody["error"] != domain.ErrEmailTaken.Error() {
		t.Fatalf("409 = %v", errBody)
	}
	want(t, srv, http.StatusBadRequest, http.MethodPost, "/api/users", map[string]any{
		"full_name": "Sin correo", "password": "secreta123"})

	want(t, srv, http.StatusNoContent, http.MethodDelete, "/api/users/"+id, nil)
	want(t, srv, http.StatusNotFound, http.MethodGet, "/api/users/"+id, nil)
	want(t, srv, http.StatusNotFound, http.MethodDelete, "/api/users/"+id, nil)
}

func TestRequestErrors(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		name, method, path string
		body               any
		status             int
		msg                string // fragmento esperado del mensaje de error
	}{
		{"id no numérico", http.MethodGet, "/api/users/abc", nil, http.StatusBadRequest, "id inválido"},
		{"id cero", http.MethodGet, "/api/rooms/0", nil, http.StatusBadRequest, "id inválido"},
		{"id fuera de rango", http.MethodGet, "/api/groups/9223372036854775808", nil, http.StatusBadRequest, "id inválido"},
		{"user_id inválido", http.MethodDelete, "/api/rooms/1/members/x", nil, http.StatusBadRequest, "user_id inválido"},
		{"presentation_id inválido", http.MethodGet, "/api/groups/1/presentations/no-es-uuid", nil, http.StatusBadRequest, "presentation_id inválido"},
		{"campo desconocido", http.MethodPost, "/api/rooms", `{"nombre":"Clase"}`, http.StatusBadRequest, `campo desconocido "nombre"`},
		{"tipo incorrecto", http.MethodPost, "/api/rooms/1/groups", `{"limit":"diez","priority":1}`, http.StatusBadRequest, "el campo limit debe ser de tipo int"},
		{"JSON mal formado", http.MethodPost, "/api/rooms", `{"name":`, http.StatusBadRequest, "JSON mal formado"},
		{"sin cuerpo", http.MethodPost, "/api/rooms", nil, http.StatusBadRequest, "falta el cuerpo"},
		{"datos de más", http.MethodPost, "/api/rooms", `{"name":"a"} {"name":"b"}`, http.StatusBadRequest, "después del objeto"},
		{"cuerpo enorme", http.MethodPost, "/api/rooms", `{"name":"` + strings.Repeat("a", 2<<20) + `"}`, http.StatusRequestEntityTooLarge, "1 MB"},
		{"ruta inexistente", http.MethodGet, "/api/salas", nil, http.StatusNotFound, "ruta no encontrada"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, data := call(t, srv, tc.method, tc.path, tc.body)
			var body map[string]string
			_ = json.Unmarshal(data, &body)
			if resp.StatusCode != tc.status || !strings.Contains(body["error"], tc.msg) {
				t.Fatalf("= %d %s; se esperaba %d con %q", resp.StatusCode, data, tc.status, tc.msg)
			}
		})
	}
}

func TestRoomsGroupsAndPresentations(t *testing.T) {
	srv := newServer(t)
	user := want(t, srv, http.StatusCreated, http.MethodPost, "/api/users", map[string]any{
		"full_name": "Ana", "email": "ana@example.com", "password": "secreta123"})
	uid := user["id"]

	r := want(t, srv, http.StatusCreated, http.MethodPost, "/api/rooms", map[string]any{
		"name": "Clase 1", "url": "https://spd.example/salas/clase-1", "access_key": "1234"})
	if r["has_access_key"] != true || r["access_key"] != nil || r["access_key_hash"] != nil {
		t.Fatalf("sala = %v", r)
	}
	rid := fmt.Sprint(r["id"])

	// Miembros (DetalleSala).
	members := "/api/rooms/" + rid + "/members"
	want(t, srv, http.StatusForbidden, http.MethodPost, members, map[string]any{"user_id": uid, "access_key": "0000"})
	m := want(t, srv, http.StatusCreated, http.MethodPost, members, map[string]any{"user_id": uid, "access_key": "1234"})
	if m["joined_at"] == nil || fmt.Sprint(m["room_id"]) != rid {
		t.Fatalf("pertenencia = %v", m)
	}
	want(t, srv, http.StatusOK, http.MethodPost, members, map[string]any{"user_id": uid, "access_key": "1234"})
	want(t, srv, http.StatusNotFound, http.MethodPost, members, map[string]any{"user_id": 9999, "access_key": "1234"})
	if _, data := call(t, srv, http.MethodGet, members, nil); !bytes.Contains(data, []byte(`"email":"ana@example.com"`)) {
		t.Fatalf("miembros = %s", data)
	}
	if _, data := call(t, srv, http.MethodGet, fmt.Sprintf("/api/users/%v/rooms", uid), nil); !bytes.Contains(data, []byte(`"name":"Clase 1"`)) {
		t.Fatalf("salas del usuario = %s", data)
	}
	want(t, srv, http.StatusNoContent, http.MethodDelete, fmt.Sprintf("%s/%v", members, uid), nil)
	want(t, srv, http.StatusNotFound, http.MethodDelete, fmt.Sprintf("%s/%v", members, uid), nil)

	// Grupos.
	resp, data := call(t, srv, http.MethodPost, "/api/rooms/"+rid+"/groups", map[string]any{"limit": 30, "priority": 1})
	var g map[string]any
	_ = json.Unmarshal(data, &g)
	gid := fmt.Sprint(g["id"])
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("Location") != "/api/groups/"+gid {
		t.Fatalf("grupo = %d %s (Location %q)", resp.StatusCode, data, resp.Header.Get("Location"))
	}
	want(t, srv, http.StatusBadRequest, http.MethodPost, "/api/rooms/"+rid+"/groups", map[string]any{"priority": 1})
	want(t, srv, http.StatusBadRequest, http.MethodPost, "/api/rooms/"+rid+"/groups", map[string]any{"limit": 0, "priority": 1})
	if got := want(t, srv, http.StatusOK, http.MethodPatch, "/api/groups/"+gid, map[string]any{"limit": 25}); got["limit"] != 25.0 || got["priority"] != 1.0 {
		t.Fatalf("PATCH grupo = %v", got)
	}

	// Presentaciones asignadas al grupo.
	list := "/api/groups/" + gid + "/presentations"
	resp, data = call(t, srv, http.MethodPost, list, map[string]any{"presentation_id": pres, "slides": []int{2, 4}})
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("Location") != list+"/"+pres || !bytes.Contains(data, []byte(`"slides":[2,4]`)) {
		t.Fatalf("asignar = %d %s (Location %q)", resp.StatusCode, data, resp.Header.Get("Location"))
	}
	want(t, srv, http.StatusConflict, http.MethodPost, list, map[string]any{"presentation_id": pres})
	want(t, srv, http.StatusBadRequest, http.MethodPost, list, map[string]any{"presentation_id": unknown[:8]})
	want(t, srv, http.StatusUnprocessableEntity, http.MethodPost, list, map[string]any{"presentation_id": unknown})
	down := want(t, srv, http.StatusServiceUnavailable, http.MethodPost, list, map[string]any{"presentation_id": downPID})
	if down["error"] != domain.ErrCatalogUnavailable.Error() {
		t.Fatalf("503 debe dar el mensaje genérico, sin detalles internos: %v", down)
	}
	if got := want(t, srv, http.StatusOK, http.MethodPatch, list+"/"+pres, map[string]any{"slides": []int{1}}); fmt.Sprint(got["slides"]) != "[1]" {
		t.Fatalf("PATCH diapositivas = %v", got)
	}
	want(t, srv, http.StatusBadRequest, http.MethodPatch, list+"/"+pres, map[string]any{"slides": []int{5}})
	want(t, srv, http.StatusOK, http.MethodGet, list+"/"+pres, nil)

	// Borrar la sala borra en cascada su grupo y la presentación asignada.
	want(t, srv, http.StatusNoContent, http.MethodDelete, "/api/rooms/"+rid, nil)
	want(t, srv, http.StatusNotFound, http.MethodGet, "/api/rooms/"+rid, nil)
	want(t, srv, http.StatusNotFound, http.MethodGet, "/api/groups/"+gid, nil)
	want(t, srv, http.StatusNotFound, http.MethodGet, list+"/"+pres, nil)
}

func TestGroupMemberEndpoints(t *testing.T) {
	srv := newServer(t)
	newUser := func(email string) any {
		return want(t, srv, http.StatusCreated, http.MethodPost, "/api/users", map[string]any{
			"full_name": email, "email": email, "password": "secreta123"})["id"]
	}
	ana, beto, caro := newUser("ana@example.com"), newUser("beto@example.com"), newUser("caro@example.com")
	rid := fmt.Sprint(want(t, srv, http.StatusCreated, http.MethodPost, "/api/rooms", map[string]any{"name": "Clase"})["id"])
	gid := fmt.Sprint(want(t, srv, http.StatusCreated, http.MethodPost, "/api/rooms/"+rid+"/groups",
		map[string]any{"limit": 2, "priority": 1})["id"])
	members := "/api/groups/" + gid + "/members"

	notInRoom := want(t, srv, http.StatusConflict, http.MethodPost, members, map[string]any{"user_id": ana})
	if notInRoom["error"] != domain.ErrRoomMembershipRequired.Error() {
		t.Fatalf("409 sin ser miembro de la sala = %v", notInRoom)
	}
	for _, u := range []any{ana, beto, caro} {
		want(t, srv, http.StatusCreated, http.MethodPost, "/api/rooms/"+rid+"/members", map[string]any{"user_id": u})
	}

	m := want(t, srv, http.StatusCreated, http.MethodPost, members, map[string]any{"user_id": ana})
	if fmt.Sprint(m["group_id"]) != gid || m["user_id"] != ana || m["joined_at"] == nil {
		t.Fatalf("integrante = %v", m)
	}
	want(t, srv, http.StatusOK, http.MethodPost, members, map[string]any{"user_id": ana})
	want(t, srv, http.StatusCreated, http.MethodPost, members, map[string]any{"user_id": beto})
	full := want(t, srv, http.StatusConflict, http.MethodPost, members, map[string]any{"user_id": caro})
	if full["error"] != domain.ErrGroupFull.Error()+" (cupo 2)" {
		t.Fatalf("409 grupo completo = %v", full)
	}
	want(t, srv, http.StatusConflict, http.MethodPatch, "/api/groups/"+gid, map[string]any{"limit": 1})
	want(t, srv, http.StatusNotFound, http.MethodPost, "/api/groups/9999/members", map[string]any{"user_id": ana})

	if _, data := call(t, srv, http.MethodGet, members, nil); !bytes.Contains(data, []byte(`"email":"beto@example.com"`)) {
		t.Fatalf("integrantes = %s", data)
	}
	if _, data := call(t, srv, http.MethodGet, fmt.Sprintf("/api/users/%v/groups", ana), nil); !bytes.Contains(data, []byte(`"id":`+gid)) {
		t.Fatalf("grupos de ana = %s", data)
	}
	want(t, srv, http.StatusNoContent, http.MethodDelete, fmt.Sprintf("%s/%v", members, beto), nil)
	want(t, srv, http.StatusNotFound, http.MethodDelete, fmt.Sprintf("%s/%v", members, beto), nil)
	want(t, srv, http.StatusCreated, http.MethodPost, members, map[string]any{"user_id": caro}) // quedó una plaza libre
}
