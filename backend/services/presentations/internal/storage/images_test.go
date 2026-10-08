package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writePages simula la salida del conversor: escribe cada página en un directorio temporal y
// devuelve sus rutas.
func writePages(t *testing.T, pages map[string][]byte) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for name, data := range pages {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestImportImagesSortsNaturallyAndMoves(t *testing.T) {
	paths := writePages(t, map[string][]byte{"page-1.png": pngOf(t, 1), "page-2.png": pngOf(t, 2), "page-10.png": pngOf(t, 10)})
	dest := t.TempDir()

	names, err := ImportImages(paths, dest, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"001.png", "002.png", "003.png"}; !slices.Equal(names, want) {
		t.Fatalf("nombres = %q", names)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "003.png"))
	if !bytes.Equal(got, pngOf(t, 10)) {
		t.Fatal("page-10 debería ser la tercera diapositiva")
	}
	for _, p := range paths {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s debería haberse movido, no copiado", filepath.Base(p))
		}
	}

	lim := testLimits()
	lim.MaxSlides = 2
	if _, err := ImportImages([]string{"a", "b", "c"}, dest, lim); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, se esperaba ErrLimitExceeded", err)
	}
}

// Las páginas renderizadas se verifican por su firma real antes de publicarse.
func TestImportImagesRejectsInvalidContent(t *testing.T) {
	if _, err := ImportImages(nil, t.TempDir(), testLimits()); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("sin páginas: err = %v, se esperaba ErrInvalidFile", err)
	}

	paths := writePages(t, map[string][]byte{"page-1.png": pngOf(t, 1), "page-2.png": []byte("<html><script>alert(1)</script></html>")})
	_, err := ImportImages(paths, t.TempDir(), testLimits())
	if !errors.Is(err, ErrInvalidFile) || !strings.Contains(err.Error(), "page-2.png") {
		t.Fatalf("imagen falsa: err = %v, se esperaba ErrInvalidFile sobre page-2.png", err)
	}
}

func TestSlideBaseNameKeepsLexicographicOrder(t *testing.T) {
	if got := slideBaseName(0, 12); got != "001" {
		t.Fatalf("slideBaseName(0, 12) = %s", got)
	}
	if got := slideBaseName(41, 1200); got != "0042" {
		t.Fatalf("slideBaseName(41, 1200) = %s: con más de 999 diapositivas hacen falta 4 dígitos", got)
	}
}
