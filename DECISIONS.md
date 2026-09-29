# DECISIONS.md

Supuestos que costaron una decisión. Léelos antes de volver a decidirlos.

## El repositorio no lleva publicaciones — ni en las pruebas

Las pruebas corren sobre HTML inventado con la estructura del real. Se decidió así para poder
publicar el código sin redistribuir nada de nadie.

**Excepción, y es funcional:** el analizador identifica tres partes de la reunión por su
encabezado (`lectura de la biblia`, `estudio bíblico de la congregación`, `relato bíblico`, en
`internal/meeting/meeting.go`). Son claves del formato, como el nombre de un campo en una API:
si se cambian, el código deja de reconocer esas partes. Por eso aparecen en el código y en el
fixture que las ejercita. El resto de los encabezados del fixture son inventados, porque el
analizador los deriva del HTML y no los compara con nada.

## El descifrado viene de sws2apps/meeting-schedules-parser

El algoritmo (AES-128-CBC con la clave derivada del MEPS id del documento, y zlib encima) está
adaptado de [sws2apps/meeting-schedules-parser](https://github.com/sws2apps/meeting-schedules-parser),
MIT. El crédito está en los dos README y se queda ahí.

## La biblioteca sigue en `~/.local/share/jwlib`

El binario se llama `pubkit`, pero la ruta por omisión no cambió: hay bibliotecas ya
sincronizadas que costarían horas de descarga. `JWPUBKIT_HOME` es el nombre actual,
`JWLIB_HOME` se sigue leyendo, y `XDG_DATA_HOME` se respeta antes que el `~/.local/share`
literal.

## SQLite en Go puro

`modernc.org/sqlite` en vez de `mattn/go-sqlite3`: sin cgo el binario es estático y cruza a
darwin y windows sin toolchain de C. Cuesta algo de velocidad en la indexación; se aceptó.

## El suelo de cobertura arranca en 45%

Medido, no aspiracional: el total ronda el 50%, con `internal/cli` en 21% y `internal/cdn` sin
cubrir. Poner el 80% de la flota habría dejado el CI rojo desde el primer empujón. La puerta va
un poco por debajo de lo medido porque el total baila cerca de un punto entre mi máquina y los
runners. Es un trinquete en `COVER_MIN` y en `ci.yml`; súbelo al cubrir la capa de comandos.

## Ninguna prueba toca la biblioteca real

Había una que abría `~/.local/share/jwlib` y comprobaba datos de una publicación concreta: se
saltaba en cualquier otra máquina, o sea que en el CI no probaba nada, y ataba el repositorio a
una biblioteca que nadie más tiene. Fuera. Lo que se prueba se construye en un `t.TempDir()`.
