package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var (
	// ErrNotFound indica que la presentación no existe.
	ErrNotFound = errors.New("no encontrado")
	// ErrInvalidFile indica un archivo corrupto, manipulado o sin imágenes válidas.
	ErrInvalidFile = errors.New("archivo inválido")
	// ErrLimitExceeded indica que se superó algún límite de tamaño o cantidad.
	ErrLimitExceeded = errors.New("límite excedido")
)

// Limits acota el trabajo que puede provocar un archivo subido (defensa frente a zip bombs).
type Limits struct {
	MaxUploadBytes int64 // tamaño del archivo subido
	MaxEntries     int   // entradas de un ZIP/PPTX, incluidas las que no son imágenes
	MaxSlides      int   // diapositivas por presentación
	MaxImageBytes  int64 // tamaño descomprimido de una imagen
	MaxTotalBytes  int64 // tamaño descomprimido de todas las imágenes
}

// imageTypes son los formatos de diapositiva admitidos. SVG queda fuera a propósito: puede
// contener scripts.
var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// IsImageName indica si el nombre tiene una extensión de imagen admitida.
func IsImageName(name string) bool {
	_, ok := imageTypes[strings.ToLower(filepath.Ext(name))]
	return ok
}

// ContentType devuelve el tipo MIME de una diapositiva según su extensión.
func ContentType(name string) string {
	if t, ok := imageTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

const sniffLen = 512 // bytes que necesita http.DetectContentType

// sniffImageExt devuelve la extensión canónica según la firma real (magic bytes) del
// contenido, sin fiarse del nombre del archivo.
func sniffImageExt(head []byte) (string, bool) {
	switch http.DetectContentType(head) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	}
	return "", false
}

// slideBaseName genera el nombre sin extensión de la diapositiva i (empezando en 0) de un total:
// 001, 002... con ancho fijo (más dígitos si hay más de 999) para que el orden lexicográfico del
// directorio coincida siempre con el orden de las diapositivas.
func slideBaseName(i, total int) string {
	width := max(3, len(strconv.Itoa(total)))
	return fmt.Sprintf("%0*d", width, i+1)
}

// WriteFile crea path (nunca sobrescribe: O_EXCL) y copia como mucho limit bytes de r. Si r
// tiene más, devuelve ErrLimitExceeded; el archivo parcial queda en el directorio de trabajo,
// que el llamador elimina.
func WriteFile(path string, r io.Reader, limit int64) (int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, fmt.Errorf("%w: el archivo supera %s", ErrLimitExceeded, FormatBytes(limit))
	}
	return n, nil
}

// ImportImages incorpora imágenes generadas en el propio servidor (p. ej. las páginas
// renderizadas de un PPTX): las ordena, verifica su contenido y las MUEVE a destDir como
// 001.png, 002.png... Origen y destino deben estar en el mismo sistema de archivos.
func ImportImages(paths []string, destDir string, lim Limits) ([]string, error) {
	switch {
	case len(paths) == 0:
		return nil, fmt.Errorf("%w: la presentación no tiene diapositivas", ErrInvalidFile)
	case len(paths) > lim.MaxSlides:
		return nil, fmt.Errorf("%w: la presentación tiene más de %d diapositivas", ErrLimitExceeded, lim.MaxSlides)
	}
	sorted := slices.Clone(paths)
	slices.SortStableFunc(sorted, func(a, b string) int {
		return NaturalCompare(filepath.Base(a), filepath.Base(b))
	})

	names := make([]string, 0, len(sorted))
	for i, p := range sorted {
		ext, err := sniffFile(p)
		if err != nil {
			return nil, err
		}
		name := slideBaseName(i, len(sorted)) + ext
		if err := os.Rename(p, filepath.Join(destDir, name)); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func sniffFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	ext, ok := sniffImageExt(head[:n])
	if !ok {
		return "", fmt.Errorf("%w: %q no es una imagen válida", ErrInvalidFile, filepath.Base(path))
	}
	return ext, nil
}

// FormatBytes formatea un tamaño para mensajes de error ("100 MB").
func FormatBytes(n int64) string {
	const mb = 1 << 20
	if n >= mb && n%mb == 0 {
		return fmt.Sprintf("%d MB", n/mb)
	}
	return fmt.Sprintf("%d bytes", n)
}
