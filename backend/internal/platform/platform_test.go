package platform

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestEnvDefaultsAndValues(t *testing.T) {
	t.Setenv("T_STR", "  valor  ")
	t.Setenv("T_INT", "42")
	t.Setenv("T_DUR", "90s")
	t.Setenv("T_LIST", "a, b,,c ")
	t.Setenv("T_EMPTY_LIST", "")
	t.Setenv("T_URL", "http://presentations:8081")

	var env Env
	if got := env.String("T_STR", "x"); got != "valor" {
		t.Fatalf("String = %q", got)
	}
	if got := env.String("T_UNSET", "defecto"); got != "defecto" {
		t.Fatalf("String por defecto = %q", got)
	}
	if got := env.Int("T_INT", 1); got != 42 {
		t.Fatalf("Int = %d", got)
	}
	if got := env.Duration("T_DUR", time.Second); got != 90*time.Second {
		t.Fatalf("Duration = %v", got)
	}
	if got := env.List("T_LIST", nil); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("List = %q", got)
	}
	if got := env.List("T_EMPTY_LIST", []string{"defecto"}); len(got) != 0 {
		t.Fatalf("una lista definida vacía debe anular el defecto: %q", got)
	}
	if got := env.List("T_UNSET", []string{"defecto"}); !slices.Equal(got, []string{"defecto"}) {
		t.Fatalf("List por defecto = %q", got)
	}
	if got := env.URL("T_URL", "http://localhost"); got.Host != "presentations:8081" {
		t.Fatalf("URL = %v", got)
	}
	if err := env.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
}

func TestEnvCollectsEveryError(t *testing.T) {
	t.Setenv("T_INT", "cero")
	t.Setenv("T_NEG", "-3")
	t.Setenv("T_DUR", "pronto")
	t.Setenv("T_URL", "presentations:8081")

	var env Env
	if env.Int("T_INT", 7) != 7 || env.Int("T_NEG", 7) != 7 || env.Duration("T_DUR", time.Minute) != time.Minute {
		t.Fatal("ante un valor inválido se debe devolver el valor por defecto")
	}
	env.URL("T_URL", "http://localhost")
	err := env.Err()
	if err == nil {
		t.Fatal("se esperaban errores")
	}
	for _, key := range []string{"T_INT", "T_NEG", "T_DUR", "T_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("el error no menciona %s: %v", key, err)
		}
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	cases := []struct {
		method, connection, upgrade string
		want                        bool
	}{
		{http.MethodGet, "Upgrade", "websocket", true},
		{http.MethodGet, "keep-alive, Upgrade", "WebSocket", true},
		{http.MethodPost, "Upgrade", "websocket", false},
		{http.MethodGet, "keep-alive", "websocket", false},
		{http.MethodGet, "Upgrade", "h2c", false},
	}
	for _, tc := range cases {
		r, _ := http.NewRequest(tc.method, "/", nil)
		r.Header.Set("Connection", tc.connection)
		r.Header.Set("Upgrade", tc.upgrade)
		if got := IsWebSocketUpgrade(r); got != tc.want {
			t.Fatalf("%+v: got %v", tc, got)
		}
	}
}

func TestCanonicalUUID(t *testing.T) {
	want := "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"
	for _, in := range []string{want, strings.ToUpper(want), "{" + want + "}", "urn:uuid:" + want} {
		if got, ok := CanonicalUUID(in); !ok || got != want {
			t.Fatalf("CanonicalUUID(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "..", "../etc/passwd", "6f1c1a52", want + "/x"} {
		if _, ok := CanonicalUUID(in); ok {
			t.Fatalf("CanonicalUUID(%q) debería fallar", in)
		}
	}
}
