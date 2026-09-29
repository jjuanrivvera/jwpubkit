# AGENTS.md — trabajar en jwpubkit

`pubkit` lee bibliotecas JWPUB: descarga publicaciones de la CDN abierta de jw.org, las
descifra y las indexa en SQLite + FTS5 para consultarlas sin conexión. Este archivo orienta a
quien contribuya, humano o agente.

## La regla que manda

**El repositorio no contiene publicaciones.** Ni texto, ni imágenes, ni audio, ni bases de
datos — tampoco en pruebas ni en fixtures. Todo dato de prueba es inventado con la *forma* del
marcado real. La única excepción son unos pocos encabezados de sección que el analizador usa
como claves del formato (`internal/meeting/meeting.go`): sin nombrarlos no puede reconocer las
partes de la reunión. Antes de añadir un fixture, pregúntate si el texto podría venir de una
publicación; si la respuesta no es un no rotundo, invéntalo.

## La puerta

**`make verify`.** Formato, `go vet`, `golangci-lint`, las pruebas y el suelo de cobertura
(`COVER_MIN`, hoy 50%, el mismo número que `.github/workflows/ci.yml`). Un cambio está hecho
cuando sale `0`. El suelo es un trinquete: súbelo cuando cubras más, nunca lo bajes.

## Dónde está cada cosa

- `internal/cdn` — descarga desde la CDN de jw.org y el catálogo de publicaciones.
- `internal/jwpub` — el formato: zip dentro de zip, SQLite dentro, y el descifrado
  AES-128-CBC + zlib del contenido de cada documento.
- `internal/store` — la biblioteca: esquema SQLite, índice FTS5 y las consultas.
- `internal/content` — de HTML de publicación a texto, markdown o JSON.
- `internal/bible` — referencias bíblicas: análisis, rangos y numeración de libros.
- `internal/meeting` — arma la reunión de la semana a partir de los documentos indexados.
- `internal/subs` — subtítulos de video.
- `internal/cli` — el árbol de cobra. Un archivo por comando.

## Reglas de la casa

- Los comentarios explican **por qué**, no qué.
- Pasa `cmd.Context()` a todo lo que haga red o SQL; nunca `context.Background()`.
- La biblioteca vive en `~/.local/share/jwlib` por compatibilidad con instalaciones
  anteriores; `JWPUBKIT_HOME` tiene prioridad y `JWLIB_HOME` sigue funcionando.
- El binario es `pubkit` y `jwlib` queda como enlace simbólico (`make install`).
- El driver de SQLite es `modernc.org/sqlite`, en Go puro: se compila sin cgo y cruza a
  darwin y windows sin toolchain.
