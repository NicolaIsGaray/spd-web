# spd-web · backend

Sincronización de presentaciones en tiempo real. Un presentador sube un `.pptx` o un `.pdf`, que el servidor convierte en una imagen por diapositiva; después controla qué diapositiva se muestra y todos los visores conectados por WebSocket la ven cambiar al instante.

## Arquitectura

Tres microservicios, cada uno dueño de un dominio y desplegable por separado:

```
 Frontend (React) ──►  gateway :8080      API pública única + CORS
                          │
        ┌─────────────────┴──────────────────┐
        │ /api/presentations/upload          │ /api/presentations/:id/control
        │ /api/presentations/:id             │ /ws/presentation/:id
        │ /api/presentations/:id/slides/:n   │
        ▼                                    ▼
  presentations :8081   ◄── HTTP ───   realtime :8082
  subida y conversión    (slide_count)  Hub: sesiones en memoria,
  PPTX/PDF → PNG                        WebSockets y control
        │
        ▼
  UPLOAD_DIR/<uuid>/001.png, 002.png…
```

| Servicio        | Dominio                                                           | Puerto |
|-----------------|-------------------------------------------------------------------|--------|
| `gateway`       | Punto de entrada: enruta cada endpoint al servicio dueño          | 8080   |
| `presentations` | Archivos: subida, conversión de PPTX y PDF a PNG, imágenes        | 8081   |
| `realtime`      | Sesiones en vivo: diapositiva actual, visores WebSocket, control  | 8082   |

Los servicios no comparten disco ni memoria: `realtime` solo conoce las presentaciones
consultando la API de `presentations`.

```
backend/
├── internal/platform/          infraestructura común: config (entorno y .env), logs, servidor  Gin, apagado
└── services/
    ├── gateway/                main.go + internal/gateway (proxy inverso, CORS)
    ├── presentations/          main.go + internal/
    │   ├── api/                handlers REST
    │   ├── presentation/       casos de uso (crear, consultar, resolver diapositiva)
    │   ├── storage/            servicio de archivos: guardado con límite, orden, publicación atómica
    │   └── convert/            PPTX → PDF (LibreOffice) → PNG (pdftoppm); un PDF va directo a pdftoppm
    └── realtime/               main.go + internal/
        ├── hub/                patrón Hub: estado y difusión, sin locks
        ├── api/                endpoint de control + bucles de lectura/escritura del WebSocket
        └── catalog/            cliente HTTP del servicio presentations
```

Cada servicio guarda su código en su propio `internal/`. El compilador de Go impide que un
servicio importe el código interno de otro, así que la frontera entre dominios no se puede
saltar por accidente.

## Puesta en marcha

Requisitos:

- Go 1.26 o superior.
- poppler (`pdftoppm`) para convertir cualquier subida y, además, LibreOffice (`soffice`) para
  los `.pptx`. Si falta alguno, las subidas que lo necesitan responden 503: sin LibreOffice,
  los `.pdf` funcionan igual.

```sh
cp .env.example .env  # configuración local (ver Configuración)
make run              # los tres servicios en paralelo; Ctrl+C los detiene
make run-gateway      # o cada uno por separado: run-presentations, run-realtime
make build            # binarios en bin/
make test             # todos los tests (los de LibreOffice y pdftoppm se omiten si no están instalados)
make test-race        # con el detector de carreras
```

El frontend solo habla con el gateway (`http://localhost:8080`). Por defecto se admite el
origen de Vite (`http://localhost:5173`) para CORS y WebSocket.

## API

Todas las rutas pasan por el gateway. Los errores siempre tienen la forma `{"error": "..."}`.

### `POST /api/presentations/upload`

Cuerpo `multipart/form-data` con el campo `file`: un `.pptx` o un `.pdf`. El formato se
deduce de la extensión, y el contenido tiene que corresponder a ella.

```sh
curl -F file=@presentacion.pptx http://localhost:8080/api/presentations/upload
curl -F file=@informe.pdf http://localhost:8080/api/presentations/upload
```

```json
{
  "presentation_id": "0b8e4c3e-7f4a-4f0e-8d55-2f8f0f6a9b22",
  "slide_count": 12,
  "slides": [
    {"slide": 1, "file": "001.png", "url": "/api/presentations/0b8e…/slides/1"}
  ]
}
```

| Estado | Motivo                                                                             |
|--------|------------------------------------------------------------------------------------|
| 201    | Creada (cabecera `Location`)                                                       |
| 400    | Contenido que no corresponde a la extensión (PPTX o PDF inválido), falta `file`    |
| 413    | Supera `MAX_UPLOAD_MB` u otro límite (diapositivas, tamaño descomprimido del PPTX) |
| 415    | Extensión distinta de `.pptx` o `.pdf` (los `.zip` de imágenes ya no se admiten)   |
| 422    | No se pudo convertir: documento dañado o cifrado, o se superó `CONVERT_TIMEOUT`    |
| 503    | El servidor no tiene pdftoppm o, para un `.pptx`, LibreOffice                      |

Cada diapositiva del PPTX, o cada página del PDF, se renderiza como PNG con el lado mayor de
`RENDER_MAX_PX` píxeles y se guarda como `001.png`, `002.png`… en el orden del documento. Un
PPTX pasa por LibreOffice (PPTX → PDF) y después por pdftoppm (PDF → PNG); un PDF va directo
a pdftoppm.

### `GET /api/presentations/:id`

Metadatos y lista de diapositivas de una presentación, con el mismo formato que la respuesta
de subida. Sirve para precargar todas las imágenes.

### `GET /api/presentations/:id/slides/:slide_id`

Devuelve la imagen. `slide_id` es el número de diapositiva, empezando en 1, o el nombre del
archivo (`003.png`). Las presentaciones son inmutables, así que la respuesta se puede cachear
indefinidamente (`Cache-Control: immutable`).

### `POST /api/presentations/:id/control`

Cambia la diapositiva actual y la difunde a todos los visores de esa presentación.

```sh
curl -X POST -H 'Content-Type: application/json' -d '{"action":"next"}' \
  http://localhost:8080/api/presentations/<id>/control
```

| Cuerpo                                | Efecto                                    |
|---------------------------------------|-------------------------------------------|
| `{"action": "next"}`                  | Siguiente (se detiene en la última)       |
| `{"action": "prev"}`                  | Anterior (se detiene en la primera)       |
| `{"slide": 3}` o `{"slide_index": 3}` | Ir a la diapositiva 3 (empieza en 1)      |
| `{"action": "goto", "slide": 3}`      | Igual que la anterior                     |

Responde 200 con el estado resultante, con el mismo formato que el mensaje del WebSocket.
Responde 400 si el comando es inválido o la diapositiva está fuera de rango, y 404 si la
presentación no existe.

### `GET /ws/presentation/:id` (WebSocket)

Al conectarse, el visor recibe de inmediato la diapositiva actual. Después recibe un mensaje
por cada cambio:

```json
{"type": "slide", "presentation_id": "0b8e…", "slide": 3, "slide_count": 12,
 "url": "/api/presentations/0b8e…/slides/3"}
```

Cada mensaje es el estado completo. Un visor lento puede saltarse estados intermedios, pero
siempre recibe el último y nunca los recibe desordenados. Los visores no necesitan enviar
nada.

Códigos de cierre que el frontend puede leer en `event.code`:

| Código | Significado                                   | ¿Reconectar?           |
|--------|-----------------------------------------------|------------------------|
| 4404   | La presentación no existe                     | No                     |
| 1001   | El servidor se está reiniciando               | Sí                     |
| 1013   | El servicio presentations no responde         | Sí, con espera         |
| 1006   | Corte de red                                  | Sí, con espera         |

## Configuración

Por variables de entorno o en un archivo `.env`. Todas tienen un valor por defecto pensado
para desarrollo local.

Al arrancar, cada servicio carga con [godotenv](https://github.com/joho/godotenv) el `.env`
de su directorio de trabajo (`backend/` si se lanza con `make`). `.env.example` es la
plantilla con lo que se suele ajustar: las URL de los servicios a los que enruta el gateway,
la carpeta de subidas (`UPLOAD_DIR`) y el tamaño máximo de archivo en MB (`MAX_UPLOAD_MB`).

- Las variables definidas en el entorno tienen prioridad sobre el archivo: un despliegue
  puede cambiar cualquier valor sin tocarlo.
- Si no hay `.env`, se usan el entorno y los valores por defecto (en producción lo normal es
  no tenerlo). Si está mal formado, el servicio no arranca e indica el error.
- Cualquier variable de la tabla puede ir en el `.env`, incluida `GIN_MODE`.


| Variable                     | Servicio             | Defecto                  |
|------------------------------|----------------------|--------------------------|
| `HTTP_ADDR`                  | todos                | `:8080` / `:8081` / `:8082` |
| `LOG_LEVEL`, `LOG_FORMAT`    | todos                | `info`, `text` (o `json`) |
| `GIN_MODE`                   | todos                | `debug` (usar `release` en producción) |
| `ALLOWED_ORIGINS`            | gateway, realtime    | `http://localhost:5173`  |
| `PRESENTATIONS_URL`          | gateway, realtime    | `http://localhost:8081`  |
| `REALTIME_URL`               | gateway              | `http://localhost:8082`  |
| `UPLOAD_DIR`                 | presentations        | `./uploads`              |
| `MAX_UPLOAD_MB`              | presentations        | `10` (tamaño máximo del archivo subido) |
| `MAX_SLIDES`                 | presentations        | `500` (diapositivas o páginas por presentación) |
| `MAX_EXTRACTED_MB`           | presentations        | `1024` (tamaño descomprimido de un PPTX) |
| `SOFFICE_BIN`, `PDFTOPPM_BIN`| presentations        | `soffice`, `pdftoppm`    |
| `CONVERT_TIMEOUT`            | presentations        | `3m`                     |
| `MAX_CONCURRENT_CONVERSIONS` | presentations        | `2`                      |
| `RENDER_MAX_PX`              | presentations        | `1920` (lado mayor del PNG) |
| `SESSION_IDLE_TTL`           | realtime             | `24h` (`0` = nunca liberar) |

`ALLOWED_ORIGINS` acepta una lista separada por comas. En producción debe contener solo el
dominio real del frontend; definida vacía, solo se admite el mismo origen.

## Decisiones de diseño

**Concurrencia en `realtime` (patrón Hub).** Todo el estado lo posee una única goroutine
(`Hub.Run`). Los handlers le envían altas, bajas y comandos por canales, así que no hay
memoria compartida ni locks. Cada visor tiene un buzón de capacidad 1 donde el último estado
sustituye al anterior. El Hub deposita en él sin bloquearse nunca, de modo que un visor
lento no frena a nadie. Por conexión hay dos goroutines: una única escritora, con plazo por
mensaje y ping cada 30 s, y una lectora, que procesa los pong y el cierre. Las consultas de
red al catálogo se hacen fuera del bucle del Hub.

**Seguridad de las subidas.** Solo se aceptan `.pptx` y `.pdf`, y el contenido se comprueba
antes de llegar al conversor. Un PPTX debe ser un paquete OOXML real (con
`ppt/presentation.xml`), con un número de partes acotado y un tamaño descomprimido declarado
que no supere `MAX_EXTRACTED_MB`, lo que frena las zip bombs. Un PDF debe empezar por la firma
`%PDF-`. El archivo se copia a disco en streaming, sin cargarlo en memoria, y la copia se corta
al superar `MAX_UPLOAD_MB`. Las imágenes que genera el conversor se verifican por sus magic
bytes antes de publicarse. Cada subida trabaja en `UPLOAD_DIR/.staging/<uuid>` y se publica con
un `rename` atómico, así que nunca se ve una presentación a medias.

**Conversión.** Convertir un PPTX directamente a PNG con LibreOffice solo exporta la primera
diapositiva. Por eso se convierte a PDF y se rasteriza cada página con `pdftoppm`. Un PDF
subido se salta LibreOffice y va directo a `pdftoppm`: se convierte más rápido y no necesita
LibreOffice instalado. Las conversiones simultáneas de ambos formatos están limitadas
(`MAX_CONCURRENT_CONVERSIONS`), cada una usa un perfil de LibreOffice propio y se renderizan
como mucho `MAX_SLIDES` + 1 páginas. Si se agota el tiempo, se mata el grupo de procesos
completo, sin dejar `soffice.bin` huérfanos.

**Apagado ordenado.** Ante SIGTERM, cada servicio deja de aceptar conexiones y espera a las
peticiones en curso. `realtime` además envía a cada visor un cierre 1001. Si un visor no lo
confirma en 3 s, cierra su conexión de forma forzada.

## Limitaciones conocidas

- **El endpoint de control no tiene autenticación.** Cualquiera que conozca el id de la
  presentación, que comparten todos los visores, puede cambiar la diapositiva. Antes de
  producción conviene emitir un token de presentador al subir y exigirlo en `/control`.
- **`realtime` guarda el estado en memoria.** Debe ejecutarse una sola instancia, o enrutar
  cada presentación siempre a la misma instancia. Para escalar en horizontal haría falta un
  bus compartido (Redis Pub/Sub, NATS).
- LibreOffice y pdftoppm procesan documentos no confiables. En producción conviene ejecutar
  `presentations` en un contenedor aislado y sin salida a internet.
