<div align="center">

# pubkit

**A local-first command-line toolkit for working with JWPUB libraries.**

[![CI](https://github.com/jjuanrivvera/jwpubkit/actions/workflows/ci.yml/badge.svg)](https://github.com/jjuanrivvera/jwpubkit/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jjuanrivvera/jwpubkit)](https://github.com/jjuanrivvera/jwpubkit/releases)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

`pubkit` downloads publications in JWPUB format from the open jw.org CDN, decrypts them and
indexes them into a local library (SQLite + FTS5) you can query offline: the week's meeting,
a passage with its notes, full-text search, whole documents, images and video subtitles.
A static Go binary with JSON output, meant to be driven by scripts. The older `jwlib` name
stays available as a symlink alias.

## Installation

Download a platform archive from [Releases](https://github.com/jjuanrivvera/jwpubkit/releases),
or build from source with Go 1.26 or newer:

```sh
make build       # bin/pubkit, static linux/amd64
make install     # ~/.local/bin/pubkit and the jwlib symlink
make test
make lint
make verify      # the gate: format, vet, lint, tests and the coverage floor
```

The library defaults to `~/.local/share/jwlib`. Override it with `--biblioteca`,
`JWPUBKIT_HOME` (preferred), or the compatible `JWLIB_HOME` variable.

## Commands

The examples below use the publication symbols and references you would actually type.
Commands that read the library need the publication synced first; `sync` is the only one
that reaches the network.

### `sync`

Fetch and index a publication, or import a file you already have:

```sh
pubkit sync mwb --issue 202609
pubkit sync nwtsty it wcg
pubkit sync --archivo ~/Downloads/mwb_S_202609.jwpub mwb --issue 202609
```

### `semana`

The week's meeting, assembled from what is indexed:

```sh
pubkit semana 2026-01-05 --json
```

### `versiculo`

A passage with its footnotes and cross references:

```sh
pubkit versiculo "Juan 3:16" --citas 40 --json
```

### `buscar`

Full-text search across the library:

```sh
pubkit buscar "cisterna" --pub it,w --limite 5
```

### `doc`

Render a document by its id:

```sh
pubkit doc 1102025901 --formato json
```

### `imagen`

List or extract the media attached to a document:

```sh
pubkit imagen 1102025901 --listar
```

### `subtitulos`

Subtitles for a video key:

```sh
pubkit subtitulos pub-jwbai_201507_1_VIDEO --formato vtt
```

### `pubs`

What the library currently holds:

```sh
pubkit pubs --json
```

### `expediente`

A compact dossier for a passage — verses, notes, references and related articles in one
JSON document:

```sh
pubkit expediente "Jer 38:1-13" --json
```

### `completion` and `version`

```sh
pubkit completion zsh > _pubkit
pubkit version
```

Most commands take `--json`. Run `pubkit <command> --help` for the full flags.

## Limits

- Nothing is bundled: every command reads the library **you** synced on **your** machine.
- `sync` and subtitle retrieval depend on jw.org being reachable.
- JWPUB layouts differ across generations; unsupported schemas or missing media can limit
  the fields available to the CLI.
- This project does not bundle or redistribute publication files, text, images, audio, or
  databases. Any content obtained from an upstream source remains subject to its owner's terms.

## Credits

The JWPUB decryption algorithm is adapted from
[sws2apps/meeting-schedules-parser](https://github.com/sws2apps/meeting-schedules-parser),
available under the MIT License.

Publication content belongs to its respective owner. This repository redistributes none of
it: the tests run on invented text in the shape of the real markup. A handful of section
headings do appear in the parser — they are the keys the format uses to mark the parts of a
meeting, and the code cannot recognise them without naming them.
