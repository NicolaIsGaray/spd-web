package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const (
	stagingDirName = ".staging"
	staleAfter     = time.Hour // un directorio de trabajo más antiguo que esto quedó abandonado
)

// Store gestiona el árbol de archivos de las presentaciones:
//
//	<root>/<uuid>/001.png, 002.png...  presentaciones publicadas (inmutables)
//	<root>/.staging/<uuid>/            trabajo en curso, en la MISMA partición que root para
//	                                   que la publicación sea un rename atómico
type Store struct {
	root    string
	staging string
}

// Open prepara el directorio raíz y elimina los directorios de trabajo abandonados.
func Open(root string, log *slog.Logger) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	s := &Store{root: abs, staging: filepath.Join(abs, stagingDirName)}
	if err := os.MkdirAll(s.staging, 0o750); err != nil {
		return nil, fmt.Errorf("crear %s: %w", s.staging, err)
	}
	if n := s.removeStale(); n > 0 {
		log.Info("directorios de trabajo abandonados eliminados", "cantidad", n)
	}
	return s, nil
}

// Root devuelve el directorio raíz absoluto.
func (s *Store) Root() string { return s.root }

// Stage crea el directorio de trabajo de una presentación nueva y devuelve su ruta.
func (s *Store) Stage(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("id inválido: %q", id)
	}
	dir := filepath.Join(s.staging, id)
	return dir, os.Mkdir(dir, 0o750) // Mkdir (no MkdirAll): falla si ya existe
}

// Publish mueve slidesDir a <root>/<id> con un rename atómico: los lectores ven la presentación
// completa o no la ven, nunca un directorio a medio escribir. Nunca sobrescribe otra existente.
func (s *Store) Publish(id, slidesDir string) error {
	if !validID(id) {
		return fmt.Errorf("id inválido: %q", id)
	}
	return os.Rename(slidesDir, filepath.Join(s.root, id))
}

// Discard elimina el directorio de trabajo de id (haya ido bien o mal la subida).
func (s *Store) Discard(id string) {
	if validID(id) {
		_ = os.RemoveAll(filepath.Join(s.staging, id))
	}
}

// ListSlides lee el directorio de la presentación y devuelve los nombres de sus diapositivas
// en orden. Los nombres tienen ancho fijo (001.png, 002.png...), así que el orden lexicográfico
// en que os.ReadDir devuelve las entradas es exactamente el orden de las diapositivas.
func (s *Store) ListSlides(id string) ([]string, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(filepath.Join(s.root, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() && IsImageName(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil, ErrNotFound
	}
	return names, nil
}

// SlidePath devuelve la ruta en disco de una diapositiva. name debe provenir de ListSlides.
func (s *Store) SlidePath(id, name string) string {
	return filepath.Join(s.root, id, name)
}

// removeStale borra directorios de trabajo abandonados (p. ej. tras una caída del proceso).
// Solo los antiguos: si varias instancias comparten volumen, uno reciente puede estar en uso.
func (s *Store) removeStale() int {
	entries, err := os.ReadDir(s.staging)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < staleAfter {
			continue
		}
		if os.RemoveAll(filepath.Join(s.staging, e.Name())) == nil {
			removed++
		}
	}
	return removed
}

// validID es defensa en profundidad: los handlers ya validan el UUID, pero el Store es la
// última barrera antes del sistema de archivos y nunca construye rutas con ids arbitrarios.
func validID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.String() == id
}
