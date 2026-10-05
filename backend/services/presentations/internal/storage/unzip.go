package storage

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ExtractImages extrae de forma segura las imágenes de un ZIP a destDir, en orden y
// renombradas como 001.png, 002.jpg... Devuelve los nombres finales en orden de diapositiva.
//
// Seguridad:
//   - Zip Slip: el nombre de cada entrada NUNCA se usa como ruta de destino, solo para ordenar;
//     los archivos se escriben con nombres generados dentro de destDir. Aun así, un ZIP con
//     rutas absolutas o con ".." se rechaza entero porque delata manipulación.
//   - Zip bomb: límites de entradas, de imágenes y de bytes descomprimidos (por imagen y en
//     total) impuestos MIENTRAS se copia, sin fiarse de los tamaños declarados en la cabecera.
//   - Contenido: se verifica la firma real de cada imagen y la extensión final se deriva del
//     contenido, no del nombre.
func ExtractImages(ctx context.Context, zipPath, destDir string, lim Limits) ([]string, error) {
	f, err := os.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return nil, fmt.Errorf("%w: no es un ZIP válido (%v)", ErrInvalidFile, err)
	}
	if len(zr.File) > lim.MaxEntries {
		return nil, fmt.Errorf("%w: el ZIP tiene %d entradas (máximo %d)", ErrLimitExceeded, len(zr.File), lim.MaxEntries)
	}

	images, err := selectImages(zr.File)
	if err != nil {
		return nil, err
	}
	if len(images) > lim.MaxSlides {
		return nil, fmt.Errorf("%w: el ZIP tiene %d imágenes (máximo %d)", ErrLimitExceeded, len(images), lim.MaxSlides)
	}

	names := make([]string, 0, len(images))
	var total int64
	for i, zf := range images {
		if err := ctx.Err(); err != nil {
			return nil, err // el cliente se fue o el servidor se apaga
		}
		budget := min(lim.MaxImageBytes, lim.MaxTotalBytes-total)
		name, n, err := extractImage(zf, destDir, slideBaseName(i, len(images)), budget)
		switch {
		case errors.Is(err, ErrLimitExceeded) && budget < lim.MaxImageBytes:
			return nil, fmt.Errorf("%w: el contenido descomprimido supera %s", ErrLimitExceeded, FormatBytes(lim.MaxTotalBytes))
		case errors.Is(err, ErrLimitExceeded):
			return nil, fmt.Errorf("%w: %q supera %s descomprimida", ErrLimitExceeded, zf.Name, FormatBytes(lim.MaxImageBytes))
		case err != nil:
			return nil, err
		}
		total += n
		names = append(names, name)
	}
	return names, nil
}

// selectImages valida los nombres de las entradas, descarta lo que no es una diapositiva y
// devuelve las imágenes en orden (ver NaturalCompare).
func selectImages(files []*zip.File) ([]*zip.File, error) {
	var images []*zip.File
	for _, zf := range files {
		if !filepath.IsLocal(strings.ReplaceAll(zf.Name, `\`, "/")) {
			return nil, fmt.Errorf("%w: el ZIP contiene una ruta no permitida: %q", ErrInvalidFile, zf.Name)
		}
		if zf.FileInfo().IsDir() || isJunk(zf.Name) || !IsImageName(zf.Name) {
			continue
		}
		images = append(images, zf)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("%w: el ZIP no contiene imágenes (png, jpg, webp o gif)", ErrInvalidFile)
	}
	slices.SortStableFunc(images, func(a, b *zip.File) int { return NaturalCompare(a.Name, b.Name) })
	return images, nil
}

// isJunk detecta metadatos que añaden algunos sistemas al comprimir: __MACOSX/, archivos
// AppleDouble (._slide1.png, que NO son imágenes), .DS_Store y cualquier ruta oculta.
func isJunk(name string) bool {
	for part := range strings.SplitSeq(strings.ReplaceAll(name, `\`, "/"), "/") {
		if part == "__MACOSX" || strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// extractImage copia UNA entrada a destDir con un nombre generado (jamás el de la entrada) y
// devuelve el nombre final (extensión según el contenido) y los bytes escritos.
func extractImage(zf *zip.File, destDir, baseName string, limit int64) (string, int64, error) {
	// Rechazo barato si la cabecera ya declara demasiado. No basta: la cabecera de un ZIP
	// malicioso puede mentir, por eso el límite se vuelve a imponer durante la copia.
	if zf.UncompressedSize64 > uint64(limit) {
		return "", 0, ErrLimitExceeded
	}
	rc, err := zf.Open()
	if err != nil {
		return "", 0, fmt.Errorf("%w: no se puede leer %q (%v)", ErrInvalidFile, zf.Name, err)
	}
	defer rc.Close()
	src := &trackedReader{r: rc}

	head := make([]byte, sniffLen)
	n, _ := io.ReadFull(src, head)
	if src.err != nil {
		return "", 0, fmt.Errorf("%w: %q está dañada (%v)", ErrInvalidFile, zf.Name, src.err)
	}
	head = head[:n]
	ext, ok := sniffImageExt(head)
	if !ok {
		return "", 0, fmt.Errorf("%w: %q no es una imagen válida", ErrInvalidFile, zf.Name)
	}

	name := baseName + ext
	written, err := WriteFile(filepath.Join(destDir, name), io.MultiReader(bytes.NewReader(head), src), limit)
	if src.err != nil {
		// Fallo al LEER del ZIP (datos corruptos, CRC incorrecto, más datos de los declarados...).
		return "", 0, fmt.Errorf("%w: %q está dañada (%v)", ErrInvalidFile, zf.Name, src.err)
	}
	if err != nil {
		return "", 0, err // límite superado o fallo al ESCRIBIR en disco
	}
	return name, written, nil
}

// trackedReader recuerda el error de lectura para distinguir un ZIP corrupto (error del
// cliente) de un fallo de escritura en disco (error del servidor).
type trackedReader struct {
	r   io.Reader
	err error
}

func (t *trackedReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		t.err = err
	}
	return n, err
}
