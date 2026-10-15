# pubkit

[![CI](https://github.com/jjuanrivvera/jwpubkit/actions/workflows/ci.yml/badge.svg)](https://github.com/jjuanrivvera/jwpubkit/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jjuanrivvera/jwpubkit)](https://github.com/jjuanrivvera/jwpubkit/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

![pubkit in action](assets/demo.gif)

`pubkit` is a command-line tool that downloads publications in JWPUB format from the open
jw.org CDN, decrypts them and indexes them into a local SQLite database. You then query that
library offline: the week's meeting, a Bible passage with its notes, full-text search, whole
documents, images and video subtitles.

It is for people who study or prepare from these publications and want fast, scriptable access
to the text on their own machine. Every command has readable output and a `--json` mode, so it
works as well in a shell script as at a prompt. It is a single static binary with no runtime
dependencies.

It also works on JW Library `.jwlibrary` backups: inspect them, merge several, and keep a
master copy. That part is documented in [docs/backup.md](docs/backup.md).

## Install

Download the archive for your platform from [Releases](https://github.com/jjuanrivvera/jwpubkit/releases)
(Linux, macOS and Windows; amd64 and arm64), unpack it and put `pubkit` on your `PATH`:

```sh
tar xzf jwpubkit_0.12.0_linux_amd64.tar.gz
install pubkit ~/.local/bin/
pubkit version
```

Or build from source with Go 1.26 or newer:

```sh
git clone https://github.com/jjuanrivvera/jwpubkit && cd jwpubkit
make build       # bin/pubkit
make install     # ~/.local/bin/pubkit (plus a `jwlib` symlink, the tool's old name)
```

Two commands need `ffmpeg` on your `PATH`: `media clip` and `media frame`. Nothing else does.

## Try it in 30 seconds

Publications are fetched in English (`E`) unless you say otherwise. For another language add
`--language S` (Spanish, say) or set it once; see [Configuration](#configuration).

```sh
pubkit sync mwb --issue 202609      # the meeting workbook, Sept-Oct 2026
pubkit week 2026-09-28              # that week's meeting, assembled
```

`sync` downloads the publication, checks it against the checksum jw.org publishes, decrypts
it and indexes it:

```text
· downloading mwb_E_202609.jwpub (3.7 MB)…
· decrypting and indexing mwb_E_202609…
✓ mwb_E_202609 · downloaded 3.7 MB in 803 ms · indexed in 276 ms
  9 documents, 304 paragraphs, 65 bible citations, 59 media, 106 extracts, 8 weeks
```

`week` then prints the week from the workbook: readings, songs, each part with its time, the
scriptures and the publications each part points to. If the week's Watchtower study article is
missing it is fetched for you (pass `--offline` to forbid that). Once something is synced, every
query works without a network.

For Bible passages, sync a Bible first. The study edition is large (about 128 MB, indexed in
under a minute) and carries the study notes:

```sh
pubkit sync nwtsty
pubkit verse "John 3:16"
```

## Everyday commands

Run these after `sync`. Add `--json` to any of them for machine-readable output.

| I want to | Command |
|---|---|
| See what the library holds | `pubkit pubs` |
| Read a passage with its notes and cross references | `pubkit verse "John 3:16"` |
| Follow marginal references out of a passage | `pubkit chain "Jer 38:6" --hops 2` |
| See what a passage connects to (who cites it, what it cites) | `pubkit graph "Jer 38:6"` |
| Gather everything around a passage into one document | `pubkit dossier "Jer 38:1-13"` |
| Search all indexed text | `pubkit search "cistern" --limit 5` |
| Read a document by id | `pubkit doc 202026255` |
| List a document's images at full resolution | `pubkit image 202026255 --list` |
| Read the week's Watchtower study article | `pubkit watchtower 2026-09-28` |
| Read the daily text | `pubkit daily` |
| Transcribe a video from its subtitles | `pubkit subtitles pub-jwb-125_4_VIDEO` |
| Find a phrase in transcripts you have fetched | `pubkit subtitles find "end of the day"` |
| Estimate reading time aloud | `pubkit reading-time --reference "Jer 38:1-13"` |

A few of these in more detail:

```sh
# Search: every hit carries its citation and a jw.org link that opens that paragraph.
pubkit search "cistern" --limit 2

# Documents come out as Markdown (the default), JSON, or plain text.
pubkit doc 202026255 --format json

# Images: list first, extract later. --list never touches the network or writes files.
pubkit image 202026255 --list

# Subtitles: fetching a transcript adds it to the set that `find` searches.
pubkit subtitles pub-jwb-125_4_VIDEO
pubkit subtitles find "end of the day" --limit 1

# Know what a sync will cost before running it.
pubkit sync it --plan
```

Library queries work offline; `sync`, `catalog`, `update-week`, subtitle retrieval and
`media clip`/`media frame` need the network.

The remaining commands (`catalog`, `media clip`, `media frame`, `places`, `update-week`, `drop`,
`completion`, `backup`) are covered in the [command reference](docs/reference.md) and in
`pubkit <command> --help`.

## Configuration

Settings come from four places. A flag beats an environment variable, which beats the settings
file, which beats the built-in default:

| Setting | Flag | Environment | File key | Default |
|---|---|---|---|---|
| Publication language | `--language` | `JWPUBKIT_LANG` | `language` | `E` |
| Library folder | `--library` | `JWPUBKIT_HOME` | `library` | `~/.local/share/jwlib` |
| Media folder (extracted images) | | `JWPUBKIT_MEDIA_DIR` | `media_dir` | `media/` in the library |
| Backup store | `--store` | `JWPUBKIT_BACKUP_STORE` | `backup_store` | `backups/` in the library |

Languages are jw.org symbols: `E` English, `S` Spanish, `F` French, and so on.

For a machine that always works in one language, write it once in `~/.config/pubkit/config`:

```ini
language = S
library = ~/.local/share/jwlib
media_dir = ~/media
```

Prefer the file to an exported variable. A variable set in a shell profile never reaches a cron
job, a service, or a program launched by another program, and nothing warns you when it does
not. `pubkit config` prints every setting with what decided it, so you can check:

```text
language   S   /home/you/.config/pubkit/config
library    …   built-in default
```

Each language gets its own database inside the library folder. `pubkit drop --language E`
removes one language safely; it reports what it would delete and removes nothing without `--yes`.

## How it works

A JWPUB file is a zip holding a `manifest.json` and a second zip with a SQLite database and the
publication's images. The text inside that database is encrypted. The key is derived from the
publication's own metadata: SHA-256 of `<language index>_<symbol>_<year>` (plus the issue number
when there is one), XORed with a fixed constant; the first 16 bytes are an AES-128 key and the
last 16 the IV. After AES-CBC decryption the content is zlib-compressed HTML.
`pubkit` decrypts each document, converts the HTML into structured blocks, and stores it in
SQLite with an FTS5 full-text index, along with verses, notes, citations, media and extracts.
Document ids are the same ones jw.org uses, so every result links back to the source paragraph.
Everything after `sync` is a query against that database. For the design decisions behind it,
see [DECISIONS.md](DECISIONS.md).

## Limits

- Nothing is bundled. Every command reads the library you synced, on your machine.
- `sync`, `catalog` and subtitle retrieval depend on jw.org being reachable.
- JWPUB layouts differ across publication generations. An unsupported schema or missing media
  can limit what a command returns.
- The media catalog covers what the official category tree and subtitle URLs expose. A missing
  search result does not prove that a video does not exist.
- Backup annotation (`backup annotate`) is experimental. Restoring a backup in JW Library
  replaces the device's personal study data, so export a fresh backup first.

## Legal notice and personal use

`pubkit` is an independent project. It is not affiliated with or endorsed by the owner of the
publications. It does not bundle or redistribute any publication, text, image, audio or
database, and its tests run on invented text shaped like the real markup. Publication content
belongs to its respective owner and remains subject to their terms. Use the tool for your own
study on material you are entitled to download, and do not republish what it extracts.

## Credits

The decryption algorithm is adapted from
[sws2apps/meeting-schedules-parser](https://github.com/sws2apps/meeting-schedules-parser)
(MIT). The way the week and its Watchtower issue are located follows the approach of
[Meeting Media Manager](https://github.com/sircharlo/meeting-media-manager); the idea was
reimplemented and no code was copied.

## License

MIT. See [LICENSE](LICENSE).
