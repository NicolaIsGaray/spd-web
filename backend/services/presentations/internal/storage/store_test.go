package storage

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

const pid = "6f1c1a52-2a0e-4b6b-9a59-6a1d8f3f1c11"

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStageAndPublish(t *testing.T) {
	s := openStore(t)
	work, err := s.Stage(pid)
	if err != nil {
		t.Fatal(err)
	}
	slides := filepath.Join(work, "slides")
	_ = os.Mkdir(slides, 0o750)
	for _, name := range []string{"002.png", "001.png", "003.jpg"} {
		_ = os.WriteFile(filepath.Join(slides, name), pngOf(t, 1), 0o600)
	}

	if _, err := s.ListSlides(pid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("antes de publicar no debe ser visible: err = %v", err)
	}
	if err := s.Publish(pid, slides); err != nil {
		t.Fatal(err)
	}
	s.Discard(pid)

	names, err := s.ListSlides(pid)
	if err != nil || !slices.Equal(names, []string{"001.png", "002.png", "003.jpg"}) {
		t.Fatalf("ListSlides = %q, %v", names, err)
	}
	if _, err := os.Stat(work); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el directorio de trabajo debería haberse eliminado")
	}

	// Publicar de nuevo con el mismo id nunca sobrescribe la presentación existente.
	work2, _ := s.Stage(pid)
	other := filepath.Join(work2, "slides")
	_ = os.Mkdir(other, 0o750)
	_ = os.WriteFile(filepath.Join(other, "001.png"), pngOf(t, 9), 0o600)
	if err := s.Publish(pid, other); err == nil {
		t.Fatal("Publish sobrescribió una presentación existente")
	}
}

func TestStoreRejectsUnsafeIDs(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"", "..", "../etc", ".staging", "6F1C1A52-2A0E-4B6B-9A59-6A1D8F3F1C11"} {
		if _, err := s.ListSlides(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("ListSlides(%q): err = %v, se esperaba ErrNotFound", id, err)
		}
		if _, err := s.Stage(id); err == nil {
			t.Fatalf("Stage(%q) debería fallar", id)
		}
	}
}

func TestOpenRemovesOnlyStaleStaging(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, stagingDirName, "abandonado")
	fresh := filepath.Join(root, stagingDirName, "en-curso")
	for _, d := range []string{stale, fresh} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * staleAfter)
	_ = os.Chtimes(stale, old, old)

	if _, err := Open(root, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el directorio abandonado debería haberse eliminado")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("un directorio reciente (posiblemente en uso) no debe eliminarse")
	}
}
