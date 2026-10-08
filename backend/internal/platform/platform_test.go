package platform

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

// writeDotEnv escribe un archivo .env temporal. godotenv define las variables con os.Setenv, así
// que antes se registran con t.Setenv (para restaurarlas al terminar) y se dejan sin definir.
func writeDotEnv(t *testing.T, content string, keys ...string) string {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDotEnv(t *testing.T) {
	path := writeDotEnv(t, "# comentario\nT_FILE=del-archivo\nexport T_QUOTED=\"con espacios\"\nT_BOTH=del-archivo\n",
		"T_FILE", "T_QUOTED")
	t.Setenv("T_BOTH", "del-entorno")

	var env Env
	if !env.LoadDotEnv(path) {
		t.Fatal("LoadDotEnv no cargó el archivo")
	}
	if got := env.String("T_FILE", ""); got != "del-archivo" {
		t.Fatalf("T_FILE = %q", got)
	}
	if got := env.String("T_QUOTED", ""); got != "con espacios" {
		t.Fatalf("T_QUOTED = %q", got)
	}
	if got := env.String("T_BOTH", ""); got != "del-entorno" {
		t.Fatalf("el entorno debe tener prioridad sobre el archivo: T_BOTH = %q", got)
	}

	if env.LoadDotEnv(filepath.Join(t.TempDir(), "no-existe.env")) {
		t.Fatal("LoadDotEnv informó de un archivo que no existe")
	}
	if err := env.Err(); err != nil {
		t.Fatalf("un .env inexistente no es un error: %v", err)
	}
}

func TestLoadDotEnvRejectsMalformedFile(t *testing.T) {
	path := writeDotEnv(t, "T_OK=1\nLINEA_SIN_VALOR\n", "T_OK")
	var env Env
	env.LoadDotEnv(path)
	if err := env.Err(); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("se esperaba un error que mencionara %s: %v", path, err)
	}
}

// Gin lee GIN_MODE al iniciarse el proceso; si solo viene en el .env, hay que aplicarlo después.
func TestLoadDotEnvAppliesGinMode(t *testing.T) {
	prev := gin.Mode()
	t.Cleanup(func() { gin.SetMode(prev) })

	var env Env
	env.LoadDotEnv(writeDotEnv(t, "GIN_MODE=release\n", gin.EnvGinMode))
	if err := env.Err(); err != nil || gin.Mode() != gin.ReleaseMode {
		t.Fatalf("modo de Gin = %q (err = %v), se esperaba release", gin.Mode(), err)
	}

	env = Env{}
	env.LoadDotEnv(writeDotEnv(t, "GIN_MODE=produccion\n", gin.EnvGinMode))
	if err := env.Err(); err == nil || !strings.Contains(err.Error(), gin.EnvGinMode) {
		t.Fatalf("un GIN_MODE inválido debe ser un error de configuración: %v", err)
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
