// Package presentation contiene los casos de uso del dominio "presentaciones": crear una
// presentación a partir de un documento subido (.pptx o .pdf), consultar sus metadatos y
// resolver la imagen de cada diapositiva.
package presentation

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"spd.web/services/presentations/internal/convert"
	"spd.web/services/presentations/internal/storage"
)

// Errores del dominio. El handler HTTP los traduce a códigos de estado.
var (
	ErrNotFound          = errors.New("presentación no encontrada")
	ErrSlideNotFound     = errors.New("diapositiva no encontrada")
	ErrUnsupportedFormat = errors.New("formato no soportado: sube un .pptx o un .pdf")

	// Errores de las capas inferiores, re-exportados para que el handler dependa solo de este paquete.
	ErrInvalidFile          = storage.ErrInvalidFile
	ErrLimitExceeded        = storage.ErrLimitExceeded
	ErrConversionFailed     = convert.ErrFailed
	ErrConverterUnavailable = convert.ErrUnavailable
)

// Converter renderiza un documento como una imagen por diapositiva (o página). Deduce el
// formato de la extensión de src, que Create garantiza que corresponde a su contenido.
type Converter interface {
	ToImages(ctx context.Context, src, outDir string, maxPages int) ([]string, error)
}

// Presentation es la vista pública de una presentación publicada.
type Presentation struct {
	ID         string  `json:"presentation_id"`
	SlideCount int     `json:"slide_count"`
	Slides     []Slide `json:"slides"`
}

// Slide describe una diapositiva y la URL pública de su imagen.
type Slide struct {
	Number int    `json:"slide"` // empieza en 1
	File   string `json:"file"`  // nombre en disco: 001.png, 002.png...
	URL    string `json:"url"`
}

// Service implementa los casos de uso. Es seguro para uso concurrente: no tiene estado mutable
// (cada subida trabaja en su propio directorio) y el Converter limita las conversiones.
type Service struct {
	store  *storage.Store
	conv   Converter
	limits storage.Limits
	log    *slog.Logger
}

// NewService crea el servicio.
func NewService(store *storage.Store, conv Converter, limits storage.Limits, log *slog.Logger) *Service {
	return &Service{store: store, conv: conv, limits: limits, log: log}
}

type sourceKind string

const (
	kindPPTX sourceKind = ".pptx"
	kindPDF  sourceKind = ".pdf"
)

// Create procesa un documento subido y publica la presentación:
//
//  1. genera un UUID y un directorio de trabajo exclusivo (<uploads>/.staging/<uuid>/);
//  2. guarda el archivo aplicando el límite de tamaño y comprueba que el contenido
//     corresponde a la extensión (un PPTX real o un PDF real);
//  3. renderiza cada diapositiva (PPTX) o página (PDF) con el Converter; las imágenes se
//     ordenan y se renombran como 001.png, 002.png...;
//  4. publica con un rename atómico a <uploads>/<uuid>/: nadie ve una presentación a medias.
//
// Pase lo que pase, el directorio de trabajo se elimina al terminar.
func (s *Service) Create(ctx context.Context, filename string, src io.Reader) (Presentation, error) {
	kind, err := detectKind(filename)
	if err != nil {
		return Presentation{}, err
	}

	id := uuid.NewString()
	work, err := s.store.Stage(id)
	if err != nil {
		return Presentation{}, fmt.Errorf("crear directorio de trabajo: %w", err)
	}
	defer s.store.Discard(id)

	srcPath := filepath.Join(work, "source"+string(kind))
	if _, err := storage.WriteFile(srcPath, src, s.limits.MaxUploadBytes); err != nil {
		return Presentation{}, err
	}
	switch kind {
	case kindPPTX:
		err = checkPPTX(srcPath, s.limits)
	case kindPDF:
		err = checkPDF(srcPath)
	}
	if err != nil {
		return Presentation{}, err
	}

	slidesDir := filepath.Join(work, "slides")
	names, err := s.render(ctx, srcPath, work, slidesDir)
	if err != nil {
		return Presentation{}, err
	}

	if err := s.store.Publish(id, slidesDir); err != nil {
		return Presentation{}, fmt.Errorf("publicar presentación: %w", err)
	}
	s.log.Info("presentación publicada", "presentation_id", id, "origen", string(kind), "slides", len(names))
	return newPresentation(id, names), nil
}

// Get devuelve los metadatos de una presentación publicada.
func (s *Service) Get(id string) (Presentation, error) {
	names, err := s.listSlides(id)
	if err != nil {
		return Presentation{}, err
	}
	return newPresentation(id, names), nil
}

// SlideFile resuelve una diapositiva por número ("3", empezando en 1) o por nombre de archivo
// ("003.png") y devuelve su ruta en disco. Solo se aceptan nombres presentes en el listado del
// directorio, así que no hay forma de salir de él (path traversal).
func (s *Service) SlideFile(id, ref string) (string, error) {
	names, err := s.listSlides(id)
	if err != nil {
		return "", err
	}
	if isDigits(ref) {
		n, err := strconv.Atoi(ref)
		if err != nil || n < 1 || n > len(names) {
			return "", ErrSlideNotFound
		}
		return s.store.SlidePath(id, names[n-1]), nil
	}
	if slices.Contains(names, ref) {
		return s.store.SlidePath(id, ref), nil
	}
	return "", ErrSlideNotFound
}

func (s *Service) listSlides(id string) ([]string, error) {
	names, err := s.store.ListSlides(id)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, ErrNotFound
	}
	return names, err
}

// render convierte el documento en imágenes dentro de <work>/render y las incorpora, ya
// ordenadas y renombradas, a slidesDir.
func (s *Service) render(ctx context.Context, srcPath, work, slidesDir string) ([]string, error) {
	renderDir := filepath.Join(work, "render")
	for _, dir := range []string{renderDir, slidesDir} {
		if err := os.Mkdir(dir, 0o750); err != nil {
			return nil, err
		}
	}
	pages, err := s.conv.ToImages(ctx, srcPath, renderDir, s.limits.MaxSlides)
	if err != nil {
		return nil, err
	}
	return storage.ImportImages(pages, slidesDir, s.limits)
}

// checkPPTX comprueba, ANTES de invocar a LibreOffice, que el archivo es un PPTX real (un ZIP
// OOXML con ppt/presentation.xml) y que no declara un tamaño descomprimido desmesurado. Falla
// rápido con un error claro y evita entregar archivos arbitrarios a LibreOffice.
func checkPPTX(path string, lim storage.Limits) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("%w: no es un PPTX válido", ErrInvalidFile)
	}
	defer zr.Close()

	if len(zr.File) > lim.MaxEntries {
		return fmt.Errorf("%w: el PPTX tiene %d entradas internas (máximo %d)", ErrLimitExceeded, len(zr.File), lim.MaxEntries)
	}
	var declared uint64
	found := false
	for _, f := range zr.File {
		if f.UncompressedSize64 > uint64(lim.MaxTotalBytes) {
			declared = f.UncompressedSize64 // basta una entrada desmesurada (y evita overflow al sumar)
			break
		}
		declared += f.UncompressedSize64
		found = found || f.Name == "ppt/presentation.xml"
	}
	if declared > uint64(lim.MaxTotalBytes) {
		return fmt.Errorf("%w: el PPTX descomprimido ocuparía más de %s", ErrLimitExceeded, storage.FormatBytes(lim.MaxTotalBytes))
	}
	if !found {
		return fmt.Errorf("%w: no es un PPTX válido (falta ppt/presentation.xml)", ErrInvalidFile)
	}
	return nil
}

// pdfMagic es la firma con la que empieza todo PDF.
var pdfMagic = []byte("%PDF-")

// checkPDF comprueba, ANTES de invocar a pdftoppm, que el archivo empieza por la firma de un
// PDF. Falla rápido con un error claro y evita entregar archivos arbitrarios al conversor.
func checkPDF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, len(pdfMagic))
	if _, err := io.ReadFull(f, head); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	if !bytes.Equal(head, pdfMagic) {
		return fmt.Errorf("%w: no es un PDF válido (no empieza por %%PDF-)", ErrInvalidFile)
	}
	return nil
}

func detectKind(filename string) (sourceKind, error) {
	switch kind := sourceKind(strings.ToLower(filepath.Ext(filename))); kind {
	case kindPPTX, kindPDF:
		return kind, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

func newPresentation(id string, names []string) Presentation {
	slides := make([]Slide, len(names))
	for i, name := range names {
		slides[i] = Slide{
			Number: i + 1,
			File:   name,
			URL:    "/api/presentaciones/" + id + "/slides/" + strconv.Itoa(i+1),
		}
	}
	return Presentation{ID: id, SlideCount: len(names), Slides: slides}
}

func isDigits(s string) bool {
	if s == "" || len(s) > 9 {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
