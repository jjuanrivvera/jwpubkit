# jwlib

CLI en Go para leer las publicaciones de los testigos de Jehová (formato JWPUB) desde una
biblioteca local, sin pasar por wol.jw.org. Lo determinista de una preparación (bajar la
Guía, armar la semana, copiar versículos textuales de la TNM, buscar, sacar imágenes en su
mejor resolución, transcribir videos) lo hace la herramienta en milisegundos; el modelo
solo piensa.

- Baja los JWPUB de la API abierta `b.jw-cdn.org/apis/pub-media` con caché y checksum MD5.
- Los descifra (AES-128-CBC + zlib; ver [Cómo funciona](#cómo-funciona)) y los indexa en
  SQLite con FTS5, en español.
- Todo comando tiene salida legible por defecto y `--json` para scripts y agentes.
- Un solo binario estático linux/amd64 (12 MB), sin cgo: SQLite en Go puro (`modernc.org/sqlite`).

## Instalación

Requiere Go 1.26 o más nuevo solo para compilar (lo piden `modernc.org/sqlite` y `golang.org/x/net`).

```bash
cd ~/repos/jwlib
make build            # bin/jwlib, estático linux/amd64
make install          # copia a ~/.local/bin/jwlib
make vps              # scp a VPS:~/.local/bin/jwlib
make test             # go test ./...
```

En otra máquina basta copiar `bin/jwlib`: no depende de librerías del sistema.

## Biblioteca local

`~/.local/share/jwlib/` (o `--biblioteca DIR`, o `JWLIB_HOME`):

| Ruta | Qué guarda |
|---|---|
| `pubs/*.jwpub` | los JWPUB descargados, tal cual (se verifican con el MD5 de pub-media) |
| `jwlib.db` | SQLite: texto descifrado, índice FTS5 por párrafo, versículos, notas, citas, medios, extractos |
| `subtitulos/*.vtt` | subtítulos ya descargados |
| `tmp/` | copia temporal de la base de un JWPUB mientras se indexa |

Con Guías, Atalayas recientes, `nwtsty`, `it`, `lff`, `wcg`, `jr`, `gl`, `lmd`, `th`, `ijwia`
y `sjj` ocupa unos 900 MB (780 MB son los JWPUB, casi todo imágenes; la base pesa 117 MB).

## Comandos

### `jwlib sync <símbolo>... [--issue AAAAMM] [--forzar]`

Baja, verifica, descifra e indexa. Si la copia local coincide con el MD5 de la CDN no hace nada.

```text
$ jwlib sync mwb --issue 202609
· descargando mwb_S_202609.jwpub (3.7 MB)…
· descifrando e indexando mwb_S_202609…
✓ mwb_S_202609 · descargado 3,7 MB en 307 ms · indexado en 34 ms
  Guía de actividades para la reunión Vida y Ministerio Cristianos (Septiembre y octubre de 2026)
  9 documentos, 304 párrafos, 72 citas bíblicas, 59 medios, 106 extractos, 8 semanas

$ jwlib sync nwtsty it wcg lmd th jr gl lff ijwia sjj
$ jwlib sync w --issue 202607          # La Atalaya de estudio de julio de 2026
$ jwlib sync w --issue 20130115        # antes de 2016 las Atalayas eran quincenales
$ jwlib sync --archivo ~/Descargas/th_S.jwpub th   # un JWPUB que ya tienes, sin red
$ jwlib pubs                           # qué hay en la biblioteca
```

### `jwlib semana [AAAA-MM-DD]`

Toma el lunes de esa semana y arma la reunión de entre semana desde la Guía y La Atalaya de
estudio de esa semana. Si falta alguna en la biblioteca, la sincroniza (salvo `--sin-red`).

```text
$ jwlib semana 2026-09-28
Semana del 28 de septiembre a 4 de octubre (lunes 2026-09-28)
Guía de actividades: docid 202026255 · mwb26 septiembre págs. 8-9 · https://wol.jw.org/es/wol/d/r4/lp-s/202026255
Lectura bíblica semanal: JEREMÍAS 38, 39 (Jer 38; 39)
Lectura del estudiante: Jer 38:1-13 · th lección 12 «Mostrar amabilidad y empatía» (docid 1102018452)
Canciones: 102 «Ayudemos a los débiles» (inicio) · 90 «Animémonos unos a otros» (medio) · 56 «Vive la verdad» (final)

TESOROS DE LA BIBLIA
  1. No dejemos de ayudarnos unos a otros (10 min.)
     Hablemos a favor de otros, como hizo Ébed-Mélec (Jer 38:7-9; w13 15/1 9 párr. 12).
     …
     → w13 15/1 pág. 9 «Sea valiente, Jehová está con usted» · docid 2013043 ¶22-22
     → w19 noviembre págs. 6-7 «Hagamos amistades fuertes antes del fin» · docid 2019640 ¶27-27 · jwlib sync w --issue 201911
     Textos: Jer 38:7-9; 39:15-18; 38:20
     Imagen: 202026255_univ_cnt_1.jpg · Ébed-Mélec habla con el rey Sedequías a favor del profeta Jeremías.
  2. Busquemos perlas escondidas (10 min.)
     ? Jer 39:6, 7. ¿Qué mala decisión tomó Sedequías, y qué pueden aprender los cabezas de familia de su mal ejemplo? (jr 92 párr. 1).
…
NUESTRA VIDA CRISTIANA
  7. “¿Quién me tocó?” (15 min.)
     ? ¿Qué podemos hacer para permitir que otros influyan positivamente en nosotros?
     Video: pub-jwb-125_4_VIDEO Anthony Griffin: “¿Quién me tocó?” (11:26)
  8. Estudio bíblico de la congregación (30 min.)
     Capítulo: 10 MOISÉS · «Tomó la mejor decisión» · wcg págs. 52-57 · docid 1102025910
     Relato bíblico: Éxodo 2:11-22; Hechos 7:22-29; Hebreos 11:24-26
     ¿Qué diría?
       ? ¿De qué maneras demostró valor Moisés durante esta etapa de su vida?
     …
Videos de la reunión (transcripción: jwlib subtitulos <clave>)
  pub-jwb-125_4_VIDEO          Anthony Griffin: “¿Quién me tocó?” (11:26) · parte 7. “¿Quién me tocó?”
  pub-jwbcov21_11_VIDEO        Sigamos el ejemplo de los que tuvieron fe. Imitemos a Moisés, no al faraón (2:28) · parte 8…
  pub-jwbai_201507_1_VIDEO     Escogí una carrera con futuro eterno (4:59) · parte 8…

LA ATALAYA DE ESTUDIO · 28 DE SEPTIEMBRE-4 DE OCTUBRE DE 2026
«Ayudemos a otros a conocer bien a Jehová» · docid 2026485 · w26 (202607) · https://wol.jw.org/es/wol/d/r4/lp-s/2026485
Texto temático: “Esto significa vida eterna: que lleguen a conocerte a ti, el único Dios verdadero” (JUAN 17:3).
Preguntas:
    1. ¿Cómo nos sentimos al ver el progreso espiritual de un estudiante de la Biblia?
    …
```

`--extractos` imprime el texto que la Guía trae de cada referencia (el párrafo de w13, jr,
lmd…). El JSON siempre lo incluye:

```bash
jwlib semana 2026-09-28 --json | jq '.secciones[].partes[] | {numero, titulo, minutos, preguntas}'
jwlib semana 2026-09-28 --json | jq -r '.secciones[].partes[].referencias[]? | select(.tipo=="publicacion") | "\(.docid) \(.ubicacion)\n\(.extracto)\n"'
jwlib semana 2026-09-28 --json | jq -r '.videos[].clave'
```

### `jwlib versiculo "<referencia>"`

Texto TNM textual de la edición de estudio (idéntico a wol carácter por carácter, espacios de
no separación incluidos), notas al pie con la palabra a la que se refieren, referencias
marginales, notas de estudio y los documentos de la biblioteca que citan el pasaje.

```text
$ jwlib versiculo "Jer 38:6"
Jeremías 38:6 · Traducción del Nuevo Mundo (edición de estudio)

Jer 38:6 Así que agarraron a Jeremías y lo arrojaron en la cisterna de Malkiya, el hijo del rey, en el Patio de la Guardia; lo bajaron con sogas. En la cisterna no había agua, solo fango, y Jeremías empezó a hundirse en el fango.

Referencias marginales
  38:6 g «Guardia» → Jer 33:1; 37:21; 38:28

Citado en 22 documentos de la biblioteca (primeros 5; usa --citas 0 para todos):
  2026484     w26     Sigue conociendo mejor a Jehová · pid 22
  202026255   mwb26   28 de septiembre a 4 de octubre · pid 2, 18
  1200000978  it      Cisterna · pid 6
  …
```

Acepta `"Jer 38:1-13"`, `"Jeremías 38"`, `"1 Cor. 13:4-7"`, `"Sal 23"` (con su encabezado),
`"Jer 38:28-39:2"`, `"Jer 38:6; 39:1, 4-6"`, `"3 Juan 3, 4"`. Opciones: `--citas N`,
`--sin-notas`, `--sin-citas`. Si la Biblia de estudio no está, la sincroniza (127 MB).

### `jwlib buscar "<consulta>" [--pub it,w,...] [--limite N] [--biblia]`

Búsqueda de texto completo. Todas las palabras en el mismo párrafo, sin importar tildes;
`"frase exacta"` y `prefijo*`. Devuelve el mejor párrafo de cada documento.

```text
$ jwlib buscar "Ébed-Mélec" --limite 3
1200001247  it      Ébed-mélec (pid 1, 2 párrafos coinciden)
            «ÉBED»-«MÉLEC»
2013043     w13     Sea valiente, Jehová está con usted (pid 44, párr. 12, 2 párrafos coinciden)
            ¿Cómo demostró valor «Ébed»-«mélec»?
…
$ jwlib buscar "fango cisterna" --biblia
Versículos (TNM):
  Jer 38:6  …En la «cisterna» no había agua, solo «fango», y Jeremías empezó a hundirse en el «fango».
```

### `jwlib doc <docid> [--formato md|json|txt]`

El documento completo. El docid es el mismo de wol (`wol.jw.org/es/wol/d/r4/lp-s/<docid>`).

- **md** (por defecto): encabezados, párrafos con el número con que se citan (`**12**`),
  preguntas como citas (`> **9, 10.** …`), imágenes con su pie, referencias a otras
  publicaciones como enlaces de wol con el docid y el párrafo, notas al pie y, al final, la
  lista de citas bíblicas y referencias.
- **txt**: las mismas marcas que usaba `totext.py` (`[H1]`, `[PREGUNTA n]`, `[IMAGEN …]`).
- **json**: bloques con pid, tipo, número, texto, citas, referencias y videos.

La numeración sigue a las publicaciones: en Perspicacia, `it “Egipto, egipcio” párr. 28` es el
párrafo que `jwlib doc 1200001265` muestra como `28`, y las entradas con sentidos numerados se
cuentan por sentido (`[núm. 2 · párr. 1]`, como en `it “Madián, madianitas” núm. 2 párrs. 1, 2`).

Si el documento no está sincronizado pero otra publicación trae un extracto (la Guía trae el
capítulo entero del libro de estudio), `doc` muestra ese extracto y dice cómo bajar el completo.

### `jwlib imagen <docid> [--salida DIR] [--media-store] [--listar]`

Para cada imagen compara la copia del JWPUB con las de `cms-imgp.jw-cdn.org` (tamaño `xl`, y
`lg` si no hay `xl`) y se queda con la de más píxeles o, a igualdad, más calidad.

```text
$ jwlib imagen 202026255 --media-store
docid 202026255 · 28 de septiembre a 4 de octubre · 4 imágenes
 1. 202026255_univ_cnt_1.jpg  1200×675  185 KB  (cdn xl) · descartadas: jwpub 1200×675 85 KB
    → /media/96/96a9cc16d6cc270c57855a28cb017de17e5e1597807214ed86bb32cb26adeffd.jpg
    Pie: Ébed-Mélec habla con el rey Sedequías a favor del profeta Jeremías.
    Descripción: Ante la mirada de los guardias del rey, Ébed-Mélec se arrodilla…
```

`--media-store` guarda en `<media-dir>/<xx>/<sha256>.<ext>` (el formato de
`jw/scripts/media-store.sh`) y muestra la ruta `/media/…` para el markdown. El `media-dir` por
defecto es `~/repos/jw/media` en el PC y `~/Repos/personal/jw/media` en el VPS (o
`--media-dir`, o `JWLIB_MEDIA_DIR`). Sin `--salida` ni `--media-store` guarda en la carpeta actual.

### `jwlib subtitulos <clave>`

Transcripción de un video desde sus subtítulos WebVTT (API mediator; si no los trae, prueba
pub-media). La clave sale de `jwlib semana`; también acepta el enlace de jw.org
(`finder?lank=…`), `webpubvid://…` y formas cortas (`jwb-125:4`, `jwbai:201507:1`).

```text
$ jwlib subtitulos pub-jwb-125_4_VIDEO
Anthony Griffin: “¿Quién me tocó?”
pub-jwb-125_4_VIDEO · 11:26 · subtítulos: https://cfp2.jw-cdn.org/a/edcf18/1/o/jwb-125_S_04.vtt

¿Se han preguntado alguna vez antes de ir a dormir algo así como “En el día de hoy, ¿quién me tocó?”? …
```

`--tiempos` da una línea por subtítulo con su marca de tiempo; `--formato vtt` el archivo tal cual.

### Opciones globales

`--json`, `--biblioteca DIR`, `--sin-red` (no descarga nada: usa solo la biblioteca y la caché
de videos), `--idioma S`, `-q/--silencioso` (sin progreso en stderr).

## Límites

- **Solo español.** El descifrado sirve para cualquier idioma, pero `semana` reconoce textos en
  español ("Canción", "mins.", "Lectura de la Biblia") y los enlaces de wol apuntan a `/es/`.
- **Solo lo que está en la biblioteca.** `buscar` y "citado en" cubren las publicaciones
  sincronizadas, no todo lo publicado. El Índice de publicaciones y la Guía de estudio de wol
  siguen siendo más amplios para referencias antiguas.
- **Publicaciones sin JWPUB.** pub-media no tiene todo: por ejemplo ¡Despertad! por número
  (`g 202301` da 404) ni las series web como `ijwbq`. Para eso sigue haciendo falta wol.
- **Videos sin subtítulos.** Las canciones (`sjjm`) y algunos videos de la Guía (`mwbv`) no
  traen subtítulos en ninguna de las dos APIs; `subtitulos` lo dice con el detalle. La letra de
  una canción está en `sjj`: `jwlib doc 1102016902`.
- **Imágenes.** La CDN de imágenes solo tiene las de nombre `<docid>_univ_…` o `_S_…` sin
  medidas en el nombre; las demás se toman del JWPUB. Para `mwb`, `w` y `wcg` la CDN da la misma
  resolución que el JWPUB (1200×675) con el doble de calidad JPEG.
- **Atalayas antiguas.** Las de 2013 o antes usan el esquema 6 de JWPUB: no traen extractos ni
  la relación imagen-párrafo, y su tabla `BibleCitation` numera los versículos según la TNM de
  1987. Por eso las citas se toman de los enlaces del texto (libro:capítulo:versículo), que valen
  para cualquier edición.
- **Dos `sync` de la misma publicación a la vez** pueden pisarse el `.part` de la descarga.
  Las lecturas en paralelo sí son seguras (SQLite en modo WAL).

## Cómo funciona

Un JWPUB es un zip con `manifest.json` y `contents`, otro zip con la base SQLite de la
publicación y sus imágenes. El texto (`Document.Content`, `BibleVerse.Content`,
`Extract.Content`, `Footnote.Content`, `VerseCommentary.Content`, `Question.Content`…) va cifrado:

1. Tarjeta de la publicación, de la tabla `Publication`: `MepsLanguageIndex_Symbol_Year`, más
   `_IssueTagNumber` si no es 0 (`1_mwb26_2026_20260900`, `1_nwtsty_2026`).
2. SHA-256 de la tarjeta XOR `11cbb5587e32846d4c26790c633da289f66fe5842a3a585ce1bc3a294af5ada7`.
3. Los primeros 16 bytes son la clave AES-128 y los otros 16 el IV. AES-CBC, relleno PKCS#7.
4. El resultado es zlib: al descomprimirlo queda el HTML.

`Document.MepsDocumentId` es el mismo docid de wol. La semana se ubica con `DatedText`
(`FirstDateOffset` = lunes en AAAAMMDD); en La Atalaya esa fila apunta al índice, cuyo enlace
lleva al artículo de estudio. Las referencias de la Guía vienen con su texto en `Extract`; las
imágenes y videos, en `Multimedia`/`DocumentMultimedia`; las referencias marginales de la Biblia,
en `BibleCitation` con `MarginalClassification = 1`, y sus letras en el HTML de `BibleChapter`.

Código: `internal/jwpub` (zip, esquema y descifrado), `internal/store` (biblioteca, índice y
consultas), `internal/content` (HTML → bloques, Markdown y texto), `internal/meeting` (semana),
`internal/bible` (libros, referencias en español y numeración de versículos), `internal/subs`
(WebVTT), `internal/cdn` (pub-media y mediator), `internal/cli` (comandos).
`scripts/comparar_wol.py` compara versículos con wol y `scripts/medir.sh` mide contra wol.

## Créditos y licencias

- **Descifrado:** el algoritmo se portó de
  [sws2apps/meeting-schedules-parser](https://github.com/sws2apps/meeting-schedules-parser),
  `src/common/jwpub_parser.ts` (MIT License, Copyright (c) 2025 Scheduling Workbox System).
  El encargo apuntaba a Meeting Media Manager, pero M³ no descifra `Document.Content`: lee las
  tablas de medios de la base, sin tocar el texto. Se comprobó en su código y en su historia
  completa (16.620 commits: ni `createDecipheriv` en código propio, ni la constante XOR, ni
  `inflate`/`pako`).
- **Semana y Atalaya:** la ubicación por `DatedText` y los desfases de 6, 8, 10 y 12 semanas para
  buscar el número de La Atalaya siguen la lógica de
  [Meeting Media Manager](https://github.com/sircharlo/meeting-media-manager)
  (`src/helpers/jw-media.ts`, AGPL-3.0). Se reimplementó la idea; no se copió código.
- El contenido de las publicaciones es © Watch Tower Bible and Tract Society of Pennsylvania.
  jwlib es para estudio personal: no redistribuye publicaciones y los tests usan JWPUB
  sintéticos con texto inventado (solo el test de descifrado incluye el blob cifrado de un
  versículo).
