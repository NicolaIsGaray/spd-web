package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCheckEmail(t *testing.T) {
	for in, want := range map[string]string{
		"ana@example.com":       "ana@example.com",
		"  Ana@Example.COM  ":   "ana@example.com",
		"ana.lopez+spd@uni.edu": "ana.lopez+spd@uni.edu",
	} {
		if got, err := checkEmail(in); err != nil || got != want {
			t.Fatalf("checkEmail(%q) = %q, %v; se esperaba %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ana", "ana@", "@example.com", "Ana <ana@example.com>", "ana@example.com, otro@example.com",
		strings.Repeat("a", 250) + "@e.com"} {
		if _, err := checkEmail(in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("checkEmail(%q): err = %v, se esperaba ErrInvalid", in, err)
		}
	}
}

func TestCheckName(t *testing.T) {
	if got, err := checkName("name", "  Sala 1 "); err != nil || got != "Sala 1" {
		t.Fatalf("checkName = %q, %v", got, err)
	}
	if _, err := checkName("name", strings.Repeat("ñ", maxNameLen)); err != nil {
		t.Fatalf("%d caracteres multibyte deben caber: %v", maxNameLen, err)
	}
	for _, in := range []string{"", "   ", strings.Repeat("a", maxNameLen+1)} {
		if _, err := checkName("name", in); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "name") {
			t.Fatalf("checkName(%q): err = %v, se esperaba ErrInvalid que mencione el campo", in, err)
		}
	}
}

func TestCheckRoles(t *testing.T) {
	got, err := checkRoles([]string{" admin", "presentador", "admin "})
	if err != nil || !slices.Equal(got, []string{"admin", "presentador"}) {
		t.Fatalf("checkRoles = %q, %v; se esperaba sin espacios ni repetidos", got, err)
	}
	if got, err := checkRoles(nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("checkRoles(nil) = %#v, %v; se esperaba una lista vacía", got, err)
	}
	for _, in := range [][]string{{"admin", " "}, {strings.Repeat("r", maxRoleLen+1)}, make([]string, maxRoles+1)} {
		if _, err := checkRoles(in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("checkRoles(%q): err = %v, se esperaba ErrInvalid", in, err)
		}
	}
}

func TestCheckURL(t *testing.T) {
	for _, in := range []string{"", "https://spd.example/salas/1", "http://localhost:5173/salas/1"} {
		if _, err := checkURL(in); err != nil {
			t.Fatalf("checkURL(%q): %v", in, err)
		}
	}
	for _, in := range []string{"salas/1", "/salas/1", "ftp://spd.example", "javascript:alert(1)", "https://", "https://x/" + strings.Repeat("a", maxURLLen)} {
		if _, err := checkURL(in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("checkURL(%q): err = %v, se esperaba ErrInvalid", in, err)
		}
	}
}

func TestHashSecret(t *testing.T) {
	hash, err := hashSecret("password", "contraseña segura", minPassword)
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("contraseña segura")) != nil {
		t.Fatal("el hash no corresponde a la contraseña")
	}
	for _, in := range []string{"corta", strings.Repeat("x", maxSecret+1)} {
		if _, err := hashSecret("password", in, minPassword); !errors.Is(err, ErrInvalid) {
			t.Fatalf("hashSecret(%d bytes): err = %v, se esperaba ErrInvalid", len(in), err)
		}
	}
	if hash, err := accessKeyHash(""); err != nil || hash != "" {
		t.Fatalf("una clave vacía significa sala sin clave: %q, %v", hash, err)
	}
}

func TestCheckGroup(t *testing.T) {
	one, zero, neg := 1, 0, -3
	for _, in := range []GroupFields{{}, {Limit: &one}, {Priority: &one}, {Limit: &one, Priority: &one}} {
		if err := checkGroup(in); err != nil {
			t.Fatalf("checkGroup(%+v): %v", in, err)
		}
	}
	for _, in := range []GroupFields{{Limit: &zero}, {Limit: &neg}, {Priority: &zero}, {Limit: &one, Priority: &neg}} {
		if err := checkGroup(in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("checkGroup(%+v): err = %v, se esperaba ErrInvalid (cupo y prioridad empiezan en 1)", in, err)
		}
	}
}
