# DECISIONS.md

Assumptions that cost a decision. Read them back instead of deciding again.

## The repository ships no publications — not even in the tests

The tests run on invented HTML shaped like the real thing. That is what makes the code
publishable without redistributing anyone's material.

**One exception, and it is functional:** the parser identifies three parts of a meeting by
their heading (`lectura de la biblia`, `estudio bíblico de la congregación`, `relato bíblico`,
in `internal/meeting/meeting.go`). Those are keys of the format, like a field name in an API:
change them and the code stops recognising those parts. That is why they appear both in the
code and in the fixture that exercises them. Every other heading in the fixture is invented,
because the parser derives those from the HTML and compares them against nothing.

## The CLI speaks English; the publications speak their own language

Commands, flags, help, errors, output labels and JSON keys are English. Text that came out of
a publication is printed as the publication wrote it — translating data would be a lie about
what is on disk.

The Spanish names the CLI shipped with are kept as aliases, and the old flag names are mapped
onto the current ones (`legacyFlags`), because scripts written against v0.1.0 should not break
over a rename.

## Book names are learned from the library, not shipped per language

English and Spanish are built in. Anything else comes from the Bible the user syncs:
`BibleBook.ChapterDisplayTitle` is stored in `book_name` at index time and registered with the
`bible` package when the library opens.

Hand-writing a 66-book table for each language jw.org publishes would be a large, stale,
error-prone dataset in a repository whose whole point is to hold no publication data. The
library already carries the names, in the exact spelling of the edition the user has.

Parsing accepts every known language at once while display follows `--language`: a reference
pasted from a Spanish article still resolves while reading an English library, and the two
spellings mean the same verse either way.

## One database per language, not one table with a language column

A MEPS document id is the same number in every language: the English and the Spanish edition of
a publication share it. With `doc.docid` as the primary key, syncing the second language
silently left the first with zero documents — measured, not theorised.

The fix is a database file per language rather than a composite key. A composite key would work
only as long as every query remembers to scope by language, forever; that is precisely the
mistake that produced the bug. Separate files make the collision impossible instead of
forbidden, cost nothing on disk (the `.jwpub` cache stays shared), and need no migration: the
pre-language `jwlib.db` keeps serving the language it already holds, and any other language
opens `jwlib.<lang>.db`. A library created from now on always uses the per-language name, so
nothing downstream has to guess what a file contains.

## The meeting is read by its structure, never by its words

A workbook is the same document in every language: the same paragraph ids, the same MEPS
document ids, and — this is the part that matters — the same untranslated class names in the
markup. Measured across Spanish, English, Japanese, Arabic, Russian and Chinese.

So the parser keys on structure:

| what | signal |
|---|---|
| the three sections | the icon class on the heading's wrapper: `dc-icon--gem`, `--wheat`, `--sheep` (and the pre-2025 `…--rev2021` form) |
| songs | a publication extract of class **31** anchored at the heading; the number is the chapter number of the songbook document it points at |
| minutes | the digits inside the brackets a part announces its length in, read in any script |
| part numbers | the digits that open the title, with only the separator after them removed |
| the student Bible reading | the last part of the opening section that carries a Bible reference of its own |
| the congregation study | the last part of the closing section whose extract parses as a chapter — the successful parse *is* the identification |
| the accounts to read | the heading group that holds Bible references and no question or answer field |

Two traps that shaped this, both measured rather than assumed:

- **`RefPublication.Symbol` is translated.** The songbook is `sjj` in Spanish and English but
  written in the local script in Arabic and Japanese. Never match a symbol; match the class.
- **Digits are not ASCII.** Arabic writes ten minutes `١٠`, Chinese brackets it `（10分）`.
  `\d`, `strconv.Atoi` and `^(\d+)\.` all fail silently there, which is why `digits.go` reads
  any Unicode decimal block and why the brackets are matched in three forms.

When the markup carries no section markers at all, that is not a language problem and is not
reported as one: the parts and their text still come out and a note says the markers were
missing. The same goes per field — a week with no congregation study (an assembly, a circuit
overseer's visit) reports that instead of inventing one.

## The JWPUB decryption comes from sws2apps/meeting-schedules-parser

The algorithm (AES-128-CBC with the key derived from the document's MEPS id, and zlib on top)
is adapted from [sws2apps/meeting-schedules-parser](https://github.com/sws2apps/meeting-schedules-parser),
MIT. The credit is in the README and stays there.

## The library stays at `~/.local/share/jwlib`

The binary is called `pubkit`, but the default path did not change: there are already-synced
libraries that would cost hours to download again. `JWPUBKIT_HOME` is the current name,
`JWLIB_HOME` is still read, and `XDG_DATA_HOME` is honoured before the literal `~/.local/share`.

## Pure-Go SQLite

`modernc.org/sqlite` rather than `mattn/go-sqlite3`: without cgo the binary is static and
cross-compiles to darwin and windows with no C toolchain. It costs some indexing speed, which
was accepted.

## The coverage floor starts at 45%

Measured, not aspirational: the total sits around 50%, with `internal/cli` near 21% and
`internal/cdn` uncovered. The fleet's 80% would have left CI red from the first push. The gate
sits a little under the measured number because the total drifts about a point between
machines. It is a ratchet in `COVER_MIN` and in `ci.yml`; raise it as the command layer gets
covered.

## No test touches a real library

There was one that opened `~/.local/share/jwlib` and checked facts from a specific
publication: it skipped on any other machine, so it proved nothing in CI, and it tied the
repository to a library nobody else has. Gone. What is tested is built in a `t.TempDir()`.
