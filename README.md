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

The library defaults to `~/.local/share/jwlib`. Override it with `--library`,
`JWPUBKIT_HOME` (preferred), or the compatible `JWLIB_HOME` variable.

## Language and settings

Publications are fetched in the language you ask for — the jw.org symbols: `E` english,
`S` spanish, `F` french, and so on:

```sh
pubkit sync w --issue 202607 --language S
```

A machine whose library is in one language says so once, in `~/.config/pubkit/config`:

```ini
# the library on this machine is in spanish
language = S
library = ~/.local/share/jwlib
media_dir = ~/media
```

The settings file rather than an exported variable, because a variable set in a shell profile
never reaches a cron job, a service, or a program started by another program — and when it
does not, nothing says so. `pubkit config` prints every setting with what decided it:

```
settings file: /home/you/.config/pubkit/config

language   S    /home/you/.config/pubkit/config
library    …    built-in default
```

A flag beats an environment variable (`JWPUBKIT_LANG`, `JWPUBKIT_HOME`, `JWPUBKIT_MEDIA_DIR`),
which beats the file, which beats the default.

Bible references are read and printed in the language of the Bible you synced: English and
Spanish are built in, and every other language is learned from the Bible itself when it is
indexed, so `pubkit verse` speaks whatever your library speaks.

One command is narrower: `week` takes the meeting workbook apart by matching the headings the
publication uses, and today it only knows how to do that in spanish. In any other language it
still lists the parts and their text, and tells you which fields came back empty.

## Commands

The examples use the publication symbols and references you would actually type. Everything
but `sync` and `subtitles` works offline, on what the library already holds.

### `sync`

Fetch and index a publication, or import a file you already have:

```sh
pubkit sync mwb --issue 202609
pubkit sync nwtsty it wcg
pubkit sync --file ~/Downloads/mwb_E_202609.jwpub mwb --issue 202609
```

### `week`

The week's meeting, assembled from what is indexed:

```sh
pubkit week 2026-01-05 --json
```

### `verse`

A passage with its footnotes and cross references:

```sh
pubkit verse "John 3:16" --citations 40 --json
```

### `search`

Full-text search across the library:

```sh
pubkit search "cistern" --pub it,w --limit 5
```

### `doc`

Render a document by its id:

```sh
pubkit doc 1102025901 --format json
```

### `image`

List or extract the media attached to a document:

```sh
pubkit image 1102025901 --list
```

### `subtitles`

Transcribe a video from its subtitles:

```sh
pubkit subtitles pub-jwbai_201507_1_VIDEO --format vtt
```

### `pubs`

What the library currently holds:

```sh
pubkit pubs --json
```

### `dossier`

Everything around a passage — verses, notes, references and the articles that cite it — in one
document meant to be read once:

```sh
pubkit dossier "Jer 38:1-13" --json
```

### `completion` and `version`

```sh
pubkit completion zsh > _pubkit
pubkit version
```

Most commands take `--json`. Run `pubkit <command> --help` for the full flags.

The spanish command and flag names this tool shipped with (`semana`, `versiculo`, `buscar`,
`imagen`, `subtitulos`, `expediente`, `--biblioteca`, `--limite`…) still work as aliases.

## Limits

- Nothing is bundled: every command reads the library **you** synced on **your** machine.
- `sync` and subtitle retrieval depend on jw.org being reachable.
- `week` understands the workbook markup of spanish publications only, for now.
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
