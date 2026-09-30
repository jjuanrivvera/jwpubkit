<div align="center">

# pubkit

**A local-first command-line toolkit for working with JWPUB libraries.**

[![CI](https://github.com/jjuanrivvera/jwpubkit/actions/workflows/ci.yml/badge.svg)](https://github.com/jjuanrivvera/jwpubkit/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jjuanrivvera/jwpubkit)](https://github.com/jjuanrivvera/jwpubkit/releases)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

![pubkit in action](assets/demo.gif)

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

## More than one Bible

A library can hold a study edition and a plain one — they carry the same verses, but the
glossary of terms ships only with the plain one. Reads come from the Bible that can answer
most, which is the one with study notes. `pubkit config` prints which database file is in use;
`pubkit drop` removes one language's library safely.

Bible references are read and printed in the language of the Bible you synced: English and
Spanish are built in, and every other language is learned from the Bible itself when it is
indexed, so `pubkit verse` speaks whatever your library speaks.

`week` reads the workbook structure through untranslated icon classes, document classes and
Unicode digits. Missing structural fields are reported rather than guessed from translated wording.

## Commands

The examples use the publication symbols and references you would actually type. Library queries work offline. Synchronization, catalog refresh and remote clips need the network.

### `sync`

Fetch and index a publication, or import a file you already have:

```sh
pubkit sync mwb --issue 202609
pubkit sync nwtsty it wcg
pubkit sync nwtsty it --plan --json
pubkit sync scl ijwbv --interval 2s --budget-bytes 50000000 --json
pubkit sync --file ~/Downloads/mwb_E_202609.jwpub mwb --issue 202609
```

`--plan` reads CDN metadata and verifies existing archive checksums without creating or changing
the library. The disk estimate is three archive sizes for a new download, not a guarantee.
`--budget-bytes` limits planned publication download bytes and reports pending symbols.

### `catalog`

```sh
pubkit catalog publications w --year 2026 --format JWPUB --interval 2s --json
pubkit catalog publications wcgr --format PDF --json
pubkit catalog media --language S --refresh --interval 2s
```

The media catalog traverses every category reachable from the mediator root, not just the
`AllVideos` category. Items are deduplicated by language-agnostic key and retain category
provenance, publication date, duration, renditions, sizes, checksums and subtitle URLs.
Metadata is cached under `<library>/catalog/media.<language>.json`; `--file` imports the same
JSON schema from a prior download. `--refresh --resume` reuses saved category responses
from an interrupted crawl; omit `--resume` for a fresh traversal. Discovery, transcript indexing and absence of VTT are
separate database states. Publication catalogs probe the specified symbol and issue or year;
historical Watchtower years include half-month issues. Empty issues are omitted, and
formats without JWPUB remain visible. No publication files are downloaded by this command.

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

Full-text search across the library. Every hit carries the citation in the form publications
use for themselves, and a link that opens that paragraph in your language:

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
pubkit image 1001070215 --offline --list --json
pubkit image 1102025901 --paragraph 3 --media-store
pubkit image 1102025901 --passage "Gen 1:1" --list --json
```

Images come from document markup and the multimedia table, including SVG figures. Each
result carries its document, publication and paragraph provenance. `--paragraph` uses the
publication's paragraph numbers; `--passage` selects figures spanning paragraphs that cite
that passage. Use either selector, or `--figure <filename>` and `--genre svg|photo`.
`--list` does not reach the CDN or write images. Local SVG image dependencies are embedded
when exporting; unavailable or external dependencies are reported on stderr.

### `subtitles`

Transcribe a video from its subtitles, and find a phrase in the transcripts you have:

```sh
pubkit subtitles pub-jwbai_201507_1_VIDEO --format vtt
pubkit subtitles find "a phrase" --limit 5
pubkit subtitles sync --catalog --no-video --resume --interval 2s --budget-bytes 100000000 --json
pubkit subtitles find "a phrase" --scope catalog --context 15s --json
```

`find` reports the video, the exact second the phrase is said, and a link that opens it
there. It searches only transcripts already fetched; fetching one adds it to the set.
`--scope catalog` searches windows of three adjacent cues from discovered media, so phrases
split across cues can match. `--context` returns surrounding cues, and hits include category,
publication date and transcript state. The first catalog search backfills the window index.

`subtitles sync` requires `--catalog --no-video`: it never downloads an MP4. Cached VTT files
are checked against advertised checksums and indexed per key, making interruption resumable.
The byte budget counts subtitle response bodies read in that run; metadata and transport
headers are excluded. Failed downloads remain recorded and make the command exit nonzero.
`--pub-media-fallback` optionally probes items lacking a mediator VTT through pub-media;
this adds requests and is disabled by default. Files without official subtitles are listed
as `no_vtt`, not silently treated as searchable.

### `media clip`

```sh
pubkit media clip pub-jwbvod26_16_VIDEO --from 08:12 --to 08:32 \
  --resolution 240p --mode exact --traffic-bytes 30000000 \
  --output-bytes 10000000 --output /tmp/fragment.mp4 --json
```

Requires `ffmpeg` and `ffprobe` on PATH. A loopback proxy forwards only single byte-range
requests and cancels ffmpeg if the origin ignores or misstates a range, or the response-body
traffic budget is exhausted. Traffic and output budgets are independent. An existing output
is never overwritten; failed or truncated output is discarded. Results report requested
start/end, effective duration and measured bytes. `copy` keeps the original codecs and can
start at a keyframe; `exact` reencodes. Output size limiting can shorten a clip, in which case
the command fails instead of publishing it. Audio frame rounding can slightly extend duration.

### `reading-time`

How long something takes to read aloud — a document, a passage, or text piped in:

```sh
pubkit reading-time 1102025901
pubkit reading-time --reference "Jer 38:1-13"
echo "the text of a talk" | pubkit reading-time
```

Words are counted where the script separates them and characters where it does not, so the
estimate holds in Chinese or Thai as well as in Spanish. The rates are assumptions and both
are overridable (`--wpm`, `--cpm`).

### `pubs`

What the library currently holds:

```sh
pubkit pubs --json
```

### `daily`, `watchtower`, `places`, `update-week`

```sh
pubkit daily                      # the daily text, from the yearly volume
pubkit watchtower 2026-09-28      # the study article, paragraph by paragraph with its question
pubkit places "Jer 38:6-39:2"            # range evidence with classification and provenance
pubkit update-week --with-references --interval 2s
pubkit update-week --dry-run      # what the week needs, without touching the network
```

`places` reports what each row is and where it came from. It does not say a place is located on
a map, because no such link exists in the data; see DECISIONS.md. Classification distinguishes
maps, general appendix figures, publication-title candidates and unclassified encyclopedia
terms. It does not infer geography from capitalized words. `--kind` filters classification;
`--figure` selects an exact figure filename. `update-week --with-references` resolves missing
publications named by the workbook's extracts; historical public editions from 2008–2015
use the `wp` API symbol even when their reference metadata uses `w`. Unresolved metadata
and failures are reported.

### `graph`

Walk the connections the publications already state, from a passage or a document — who quotes
it, what its margin points at, what is quoted alongside it, which videos the citing articles
embed, which terms its notes define:

```sh
pubkit graph "Jer 38:6"
pubkit graph 1102025901 --json
pubkit graph "Ps 23:1" --only cited-alongside --per-relation 40
```

Every edge names the table it came from, so an answer can be checked. Nothing is inferred.

### `chain`

Follow the marginal references out of a passage, as far as you ask, with each verse's text:

```sh
pubkit chain "Jer 38:6" --hops 2
```

The walk is breadth-first and visits a verse once, so each one is reported at the fewest
references it is reachable by — and so the walk finishes, which a naive one does not: the
references point both ways.

### `dossier`

Everything around a passage — verses, notes, references and the articles that cite it — in one
document meant to be read once:

```sh
pubkit dossier "Jer 38:1-13" --json
```

### `drop`

Remove one language's indexed library. It reports first and removes nothing without `--yes`,
and it takes the database's `-wal` and `-shm` with it, which is the part that is easy to get
wrong by hand:

```sh
pubkit drop --language E
pubkit drop --language E --cache --yes
```

### `config`

Every setting in effect and what decided it:

```sh
pubkit config
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
- Catalog coverage is limited to metadata reachable through the official category tree and
  subtitle URLs actually provided; a missing search result does not prove a video is absent.
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
