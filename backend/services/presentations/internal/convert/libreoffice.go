// Package convert renderiza documentos (PPTX y PDF) como una imagen PNG por diapositiva o
// página: los PPTX pasan por LibreOffice en modo headless y los PDF van directos a pdftoppm.
package convert

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrUnavailable indica que faltan los binarios necesarios en el servidor.
	ErrUnavailable = errors.New("conversión no disponible: el servidor no tiene LibreOffice o pdftoppm")
	// ErrFailed indica que la conversión falló o superó el tiempo máximo.
	ErrFailed = errors.New("no se pudo convertir el documento")
)

// Options configura el conversor.
type Options struct {
	SofficeBin    string        // binario de LibreOffice ("soffice")
	PdftoppmBin   string        // binario de poppler ("pdftoppm")
	Timeout       time.Duration // tiempo máximo por conversión, incluida la espera en cola
	MaxConcurrent int           // conversiones simultáneas
	MaxPixels     int           // lado mayor de cada imagen renderizada, en píxeles
}

// LibreOffice convierte documentos en imágenes en dos pasos:
//
//  1. soffice --convert-to pdf: LibreOffice renderiza todas las diapositivas del PPTX.
//     (Convertir directamente a PNG solo exporta la PRIMERA diapositiva, por eso se pasa por
//     PDF.) Un documento que ya es PDF se salta este paso.
//  2. pdftoppm -png: rasteriza cada página del PDF como una imagen.
//
// Es seguro para uso concurrente.
type LibreOffice struct {
	opts        Options
	profileRoot string

	// slots es a la vez un semáforo (limita las conversiones simultáneas, que consumen mucha
	// CPU y memoria) y un pool de perfiles de usuario: dos procesos de LibreOffice sobre el
	// mismo perfil se bloquean entre sí, así que cada conversión usa un perfil exclusivo.
	slots chan string
}

// NewLibreOffice crea el conversor y su directorio temporal de perfiles.
func NewLibreOffice(opts Options) (*LibreOffice, error) {
	if opts.MaxConcurrent < 1 {
		opts.MaxConcurrent = 1
	}
	root, err := os.MkdirTemp("", "spd-libreoffice-*")
	if err != nil {
		return nil, err
	}
	slots := make(chan string, opts.MaxConcurrent)
	for i := range opts.MaxConcurrent {
		slots <- filepath.Join(root, "profile-"+strconv.Itoa(i))
	}
	return &LibreOffice{opts: opts, profileRoot: root, slots: slots}, nil
}

// Check comprueba que los binarios necesarios están instalados: pdftoppm para cualquier
// documento y, además, LibreOffice para los PPTX.
func (lo *LibreOffice) Check() error {
	if _, err := exec.LookPath(lo.opts.PdftoppmBin); err != nil {
		return fmt.Errorf("%w: no se podrá convertir ningún documento (%v)", ErrUnavailable, err)
	}
	if _, err := exec.LookPath(lo.opts.SofficeBin); err != nil {
		return fmt.Errorf("%w: solo se podrán convertir PDF (%v)", ErrUnavailable, err)
	}
	return nil
}

// Close elimina los perfiles temporales. Llamar cuando no queden conversiones en curso.
func (lo *LibreOffice) Close() error {
	return os.RemoveAll(lo.profileRoot)
}

// ToImages renderiza cada diapositiva o página de src como PNG dentro de outDir y devuelve las
// rutas generadas (sin orden garantizado). El formato se deduce de la extensión: un .pdf va
// directo a pdftoppm y cualquier otro documento (.pptx) pasa antes por LibreOffice. Renderiza
// como mucho maxPages+1 páginas: suficiente para que el llamador detecte que se superó el
// límite sin trabajar de más.
func (lo *LibreOffice) ToImages(ctx context.Context, src, outDir string, maxPages int) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, lo.opts.Timeout)
	defer cancel()

	var profile string
	select {
	case profile = <-lo.slots:
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: demasiadas conversiones en curso (%v)", ErrFailed, ctx.Err())
	}
	defer func() { lo.slots <- profile }()
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, err
	}

	// Paso 1, solo si no es ya un PDF: PPTX → PDF.
	pdf := src
	if !strings.EqualFold(filepath.Ext(src), ".pdf") {
		var err error
		if pdf, err = lo.toPDF(ctx, profile, src, outDir); err != nil {
			return nil, err
		}
	}

	// Paso 2: PDF → una imagen PNG por página (page-1.png, page-2.png...).
	prefix := filepath.Join(outDir, "page")
	err := lo.run(ctx, profile, lo.opts.PdftoppmBin,
		"-png", "-scale-to", strconv.Itoa(lo.opts.MaxPixels),
		"-l", strconv.Itoa(maxPages+1), pdf, prefix)
	if err != nil {
		return nil, err
	}
	pages, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: el documento no tiene páginas", ErrFailed)
	}
	return pages, nil
}

// toPDF convierte src en PDF con LibreOffice y devuelve la ruta del PDF generado en outDir.
func (lo *LibreOffice) toPDF(ctx context.Context, profile, src, outDir string) (string, error) {
	err := lo.run(ctx, profile, lo.opts.SofficeBin,
		"--headless", "--norestore", "--nolockcheck", "--nodefault", "--nofirststartwizard",
		"-env:UserInstallation="+fileURL(profile),
		"--convert-to", "pdf", "--outdir", outDir, src)
	if err != nil {
		return "", err
	}
	pdf := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))+".pdf")
	if _, err := os.Stat(pdf); err != nil {
		return "", fmt.Errorf("%w: LibreOffice no generó el PDF", ErrFailed)
	}
	return pdf, nil
}

// run ejecuta un binario externo ligado a ctx: si ctx se cancela (timeout, cliente que se va,
// apagado del servidor) se mata TODO el grupo de procesos (ver killProcessGroupOnCancel).
func (lo *LibreOffice) run(ctx context.Context, home, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	// HOME apunta al perfil: en contenedores sin HOME escribible LibreOffice falla sin avisar.
	cmd.Env = append(os.Environ(), "HOME="+home)
	out := &tailBuffer{max: 2 << 10}
	cmd.Stdout, cmd.Stderr = out, out
	killProcessGroupOnCancel(cmd)
	// Si un proceso nieto heredó la salida y sigue vivo, no esperar indefinidamente.
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Run(); err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			return fmt.Errorf("%w (%v)", ErrUnavailable, err)
		case ctx.Err() != nil:
			return fmt.Errorf("%w: tiempo agotado (%v)", ErrFailed, ctx.Err())
		default:
			return fmt.Errorf("%w: %s: %v: %s", ErrFailed, filepath.Base(bin), err, out)
		}
	}
	return nil
}

func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

// tailBuffer conserva solo los últimos max bytes escritos: la parte útil de un error.
type tailBuffer struct {
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.max; over > 0 {
		b.buf = append(b.buf[:0], b.buf[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string { return strings.TrimSpace(string(b.buf)) }
