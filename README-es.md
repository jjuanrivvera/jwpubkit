# pubkit

`pubkit` descarga publicaciones en formato JWPUB desde la CDN abierta de jw.org, las descifra
y las indexa en una biblioteca local (SQLite + FTS5) para consultarla sin conexión: la reunión
de la semana, un pasaje con sus notas, búsquedas de texto completo, documentos enteros,
imágenes y subtítulos de videos. Binario estático de Go con salida JSON, pensado para que lo
manejen scripts. El nombre anterior, `jwlib`, sigue disponible como alias simbólico.

## Instalación

Descarga un archivo para tu plataforma desde [Releases](https://github.com/jjuanrivvera/jwpubkit/releases)
o compila con Go 1.26 o posterior:

```sh
make build       # bin/pubkit, linux/amd64 estático
make install     # ~/.local/bin/pubkit y el enlace jwlib
make test
make lint
make verify      # la puerta: formato, vet, lint, tests y el suelo de cobertura
```

La biblioteca predeterminada es `~/.local/share/jwlib`. Se puede cambiar con `--biblioteca`,
`JWPUBKIT_HOME` (prioritario) o `JWLIB_HOME`.

## Comandos

```sh
pubkit sync mwb --issue 202609          # descarga e indexa
pubkit sync nwtsty it wcg
pubkit semana 2026-01-05 --json         # la reunión de la semana
pubkit versiculo "Juan 3:16" --citas 40 --json
pubkit buscar "cisterna" --pub it,w --limite 5
pubkit doc 1102025901 --formato json
pubkit imagen 1102025901 --listar
pubkit subtitulos pub-jwbai_201507_1_VIDEO --formato vtt
pubkit pubs --json                      # qué hay en la biblioteca
pubkit expediente "Jer 38:1-13" --json  # el pasaje con todo alrededor
pubkit version
```

Consulta `pubkit <comando> --help` para ver todas las opciones. Salvo `sync` y los subtítulos,
todo se resuelve con lo que ya tienes en disco. Los esquemas JWPUB varían entre generaciones,
así que un archivo antiguo puede exponer menos campos.

## Créditos y límites de distribución

El algoritmo de descifrado JWPUB está adaptado de
[sws2apps/meeting-schedules-parser](https://github.com/sws2apps/meeting-schedules-parser),
bajo licencia MIT.

El contenido de las publicaciones pertenece a su propietario. Este repositorio no redistribuye
nada de eso: no incluye publicaciones, textos, imágenes, audio ni bases de datos, y las pruebas
corren sobre texto inventado con la forma del marcado real. Sí aparecen en el analizador unos
pocos encabezados de sección: son las claves con las que el formato marca las partes de la
reunión, y el código no puede reconocerlas sin nombrarlas.
