// Package api expone el dominio "presentaciones" por HTTP: subida, metadatos e imágenes.
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/presentations/internal/presentation"
	"spd.web/services/presentations/internal/storage"
)

// multipartSlack es el margen sobre el tamaño máximo del archivo para las cabeceras y los
// delimitadores del cuerpo multipart.
const multipartSlack = 1 << 20

var errBadRequest = errors.New("petición inválida")

// Handler agrupa los endpoints REST del servicio.
type Handler struct {
	svc            *presentation.Service
	maxUploadBytes int64
	log            *slog.Logger
}

// New crea el Handler.
func New(svc *presentation.Service, maxUploadBytes int64, log *slog.Logger) *Handler {
	return &Handler{svc: svc, maxUploadBytes: maxUploadBytes, log: log}
}

// Register registra las rutas del servicio.
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/api/presentaciones/upload", h.upload)
	r.GET("/api/presentaciones/:id", h.get)
	r.GET("/api/presentaciones/:id/slides/:slide_id", h.slide)
}

// upload atiende POST /api/presentaciones/upload (multipart/form-data, campo "file").
func (h *Handler) upload(c *gin.Context) {
	// Tope duro para TODO el cuerpo. El límite exacto del archivo lo aplica el servicio al
	// guardarlo en disco.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+multipartSlack)

	part, err := filePart(c.Request)
	if err != nil {
		h.fail(c, err)
		return
	}
	defer part.Close()

	body := &uploadReader{r: part}
	p, err := h.svc.Create(c.Request.Context(), part.FileName(), body)
	if err != nil {
		if body.err != nil {
			err = body.err // la causa real fue leer la subida (cliente), no el disco
		}
		h.fail(c, err)
		return
	}
	c.Header("Location", "/api/presentaciones/"+p.ID)
	c.JSON(http.StatusCreated, p)
}

// filePart recorre el cuerpo multipart hasta el campo "file" y lo devuelve como stream, SIN
// cargarlo en memoria ni en archivos temporales: el servicio lo copia directamente a su
// directorio de trabajo (en el volumen de uploads, no en un /tmp que podría ser pequeño).
func filePart(r *http.Request) (*multipart.Part, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf(`%w: se esperaba multipart/form-data con el campo "file"`, errBadRequest)
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf(`%w: falta el campo "file"`, errBadRequest)
		}
		if err != nil {
			return nil, uploadError(err)
		}
		if part.FormName() == "file" && part.FileName() != "" {
			return part, nil
		}
		_ = part.Close()
	}
}

// uploadReader recuerda el error al leer el cuerpo de la subida para distinguir un cuerpo
// truncado o demasiado grande (error del cliente) de un fallo al escribir en disco.
type uploadReader struct {
	r   io.Reader
	err error
}

func (u *uploadReader) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		u.err = uploadError(err)
	}
	return n, err
}

// uploadError clasifica un error de lectura del cuerpo: si se superó el tope, se conserva el
// *http.MaxBytesError (413); cualquier otro es un cuerpo malformado o truncado (400).
func uploadError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return err
	}
	return fmt.Errorf("%w: no se pudo leer el cuerpo multipart (%v)", errBadRequest, err)
}

// get atiende GET /api/presentaciones/:id: metadatos y lista de diapositivas (la "carpeta").
func (h *Handler) get(c *gin.Context) {
	id, ok := platform.CanonicalUUID(c.Param("id"))
	if !ok {
		platform.WriteError(c, http.StatusBadRequest, "id de presentación inválido")
		return
	}
	p, err := h.svc.Get(id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// slide atiende GET /api/presentaciones/:id/slides/:slide_id. slide_id es el número de
// diapositiva (1, 2, 3...) o el nombre del archivo (001.png).
func (h *Handler) slide(c *gin.Context) {
	id, ok := platform.CanonicalUUID(c.Param("id"))
	if !ok {
		platform.WriteError(c, http.StatusBadRequest, "id de presentación inválido")
		return
	}
	path, err := h.svc.SlideFile(id, c.Param("slide_id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		h.fail(c, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		h.fail(c, err)
		return
	}

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", storage.ContentType(path))
	hdr.Set("X-Content-Type-Options", "nosniff")
	// Una presentación publicada es inmutable: cada imagen se puede cachear indefinidamente,
	// lo que evita que cien visores descarguen la misma diapositiva en cada cambio.
	hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), info.ModTime(), f)
}

// fail traduce los errores del dominio a respuestas HTTP. A los 5xx no se les adjunta el
// detalle (puede contener rutas internas): se registra en el log.
func (h *Handler) fail(c *gin.Context, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		platform.WriteError(c, http.StatusRequestEntityTooLarge,
			"el archivo supera el máximo permitido ("+storage.FormatBytes(h.maxUploadBytes)+")")
	case errors.Is(err, presentation.ErrLimitExceeded):
		platform.WriteError(c, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, presentation.ErrUnsupportedFormat):
		platform.WriteError(c, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, presentation.ErrInvalidFile), errors.Is(err, errBadRequest):
		platform.WriteError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, presentation.ErrNotFound), errors.Is(err, presentation.ErrSlideNotFound):
		platform.WriteError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, presentation.ErrConverterUnavailable):
		h.log.Error("conversión de PPTX no disponible", "err", err)
		platform.WriteError(c, http.StatusServiceUnavailable, presentation.ErrConverterUnavailable.Error())
	case errors.Is(err, presentation.ErrConversionFailed):
		h.log.Warn("conversión de PPTX fallida", "err", err)
		platform.WriteError(c, http.StatusUnprocessableEntity, presentation.ErrConversionFailed.Error())
	case errors.Is(err, context.Canceled):
		platform.WriteError(c, http.StatusServiceUnavailable, "petición cancelada")
	default:
		h.log.Error("error interno", "path", c.Request.URL.Path, "err", err)
		platform.WriteError(c, http.StatusInternalServerError, "error interno")
	}
}
