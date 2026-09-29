# AGENTS.md — working on jwpubkit

`pubkit` reads JWPUB libraries: it downloads publications from the open jw.org CDN, decrypts
them and indexes them into SQLite + FTS5 so they can be queried offline. This file orients
whoever contributes, human or agent.

## The rule that governs

**The repository holds no publications.** No text, no images, no audio, no databases — not in
tests and not in fixtures either. Every test fixture is invented, in the *shape* of the real
markup. The one exception is a handful of section headings the parser uses as keys of the
format (`internal/meeting/meeting.go`): without naming them it cannot tell the parts of the
meeting apart. Before adding a fixture, ask whether the text could have come from a
publication; if the answer is anything but a flat no, invent it.

## The gate

**`make verify`.** Formatting, `go vet`, `golangci-lint`, `gosec`, the tests and the coverage
floor (`COVER_MIN`, the same number as `.github/workflows/ci.yml`). A change is done when it
exits `0`. The floor is a ratchet: raise it as coverage grows, never lower it.

## Language

The CLI speaks English: commands, flags, help, errors, output labels and JSON keys. What comes
out of a publication stays in the publication's language — that is data, not interface.

Publications are fetched in the language of `--language` (jw.org symbols: `E`, `S`, `F`…),
default `E`. A machine sets its own in `~/.config/pubkit/config` (`internal/config`); the
environment still overrides the file, and a flag overrides both. `pubkit config` prints what
is in effect and why — reach for it before guessing why a command behaved oddly.

Two layers know about languages, and they are not the same:

- **Book names.** English and Spanish ship built in. Every other language is *learned*: when a
  Bible is indexed, `BibleBook.ChapterDisplayTitle` is stored in `book_name` and handed to the
  `bible` package on open. References parse in any known language and print in the chosen one.
- **Workbook markup.** `internal/meeting` reads a week by its structure, never by its words: the
  untranslated icon classes on section headings, the document class of an extract, and Unicode
  digits in any script. See DECISIONS.md for the signal table. If you are ever tempted to add a
  `strings.Contains` against a translated phrase, that is the thing this was built to remove.

The Spanish command and flag names the CLI shipped with are kept as aliases
(`internal/cli/root.go`), so scripts written against an older version keep working.

## Where things live

- `internal/cdn` — downloads from the jw.org CDN and the publication catalogue.
- `internal/jwpub` — the format: a zip inside a zip, SQLite inside that, and the AES-128-CBC +
  zlib decryption of each document's content.
- `internal/store` — the library: SQLite schema, the FTS5 index and the queries. **Never judge a
  database by the size of its `.db`**: in WAL mode the content is in the `-wal`, and the three
  files (`.db`, `-wal`, `-shm`) are one unit — `Store.Files()` and `pubkit drop` treat them that
  way. **One database file per language** (`jwlib.<lang>.db`, or `jwlib.db` for a library that predates this),
  because MEPS document ids are shared across languages and would otherwise collide. The
  `.jwpub` cache under `pubs/` is shared: its filenames already carry the language.
- `internal/content` — publication HTML to text, markdown or JSON.
- `internal/bible` — Bible references: parsing, ranges, book names and verse numbering.
- `internal/meeting` — assembles the week's meeting from the indexed documents.
- `internal/subs` — video subtitles.
- `internal/cli` — the cobra tree. One file per command.

## House rules

- Comments explain **why**, not what.
- Pass `cmd.Context()` into anything that touches the network or SQL; never `context.Background()`.
- The library lives at `~/.local/share/jwlib` for compatibility with earlier installs;
  `JWPUBKIT_HOME` wins and `JWLIB_HOME` still works.
- The binary is `pubkit`, with `jwlib` as a symlink (`make install`).
- The SQLite driver is `modernc.org/sqlite`, pure Go: it builds without cgo and cross-compiles
  to darwin and windows with no C toolchain.
