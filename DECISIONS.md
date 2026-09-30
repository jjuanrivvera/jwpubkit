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

## Settings live in a file, not in the environment

A machine's language was set with `export JWPUBKIT_LANG=S` in a shell profile. Cron, systemd
and a program launched by another program never read that profile, so the variable arrived
empty and a Spanish library downloaded an English workbook. Nothing in the output said so,
because there was nothing wrong to report: the tool did exactly what an unset variable means.

Hence `~/.config/pubkit/config`, read wherever the process runs, and `pubkit config`, which
prints each setting beside what decided it. Environment variables still work and still win
over the file — they are how you override one run — but they are no longer the only way to
make a choice stick.

The lesson for testing, too: the verification that missed this ran `export JWPUBKIT_LANG=S`
in the same command as the check, which guaranteed the result it was supposed to be testing.

## The dossier's term list is not a place list, and no longer says it is

It was labeled "places". Measured on one chapter: of seven candidates, three were the names
of publications picked out of citations, and seven real places in the same notes were missed.
Nothing in any publication marks an article as being about a place — an article on a city and
an article on an abstract noun carry the same class and type — so the label was a claim the
data cannot support. It is now "terms with an article of their own", which is what the
heuristic actually finds, with its own limitations stated in the output.

The lookup itself should move to the encyclopedia's own topic table (6 444 alias-aware entries,
present in every language it is published in) rather than exact title matching. That is a
separate, additive piece of work.

## A question is evidence, not a requirement

The all-words search returned nothing for five of six questions someone actually asked while
preparing — "¿Cómo consolar a alguien que perdió a un ser querido?" has no paragraph containing
every one of its words, and no publication is written to. Measured before and after on the real
library: 0, 6, 0, 0, 0, 0 became 6, 6, 6, 6, 6, 6, with an article leading every one of them.

So the words are taken as evidence: those that narrow anything down are searched for, a
paragraph matching most of them counts, and the ranking prefers prose. `--all-words` keeps the
old behaviour for the cases that want it.

**Which words narrow anything down is measured, not listed.** A term matching more than a fifth
of the library's paragraphs cannot tell two of them apart, and that is checked against the
library — so no stopword list is carried for any of the thousand languages jw.org publishes in.
The rule needs data to mean something: under a few hundred paragraphs it stays out of the way,
because a word in two of three paragraphs is a small library, not a stopword. Getting that wrong
made the search return nothing at all, which is how it was found.

**Index entries and covers are demoted, not dropped.** The noise in an OR search is
navigational: tables of contents, covers, word indexes, the per-month container of a daily-text
publication. They are identified by MEPS document class, which is the same in every language,
and pushed below prose while staying visible and labelled — because "which index entry covers
this" is a fair question, just not the usual one.

## The graph was already in the library; what was missing was a way to walk it

Publications state their own links: a citation, a marginal reference, an extract of one work
inside another, a video embedded in an article, a term a study note defines. `pubkit graph`
walks them from a passage or a document, and **every edge names the table it came from**, so an
answer can be checked instead of trusted. Nothing is inferred and no model is involved.

The one edge nobody writes down is co-citation — the passages quoted in the same paragraph as
this one — and it is the most useful: two verses taught together forty times are being taught
together. It comes from joining the citation table to itself.

Caps are per relation, because a much-quoted verse has hundreds of citing documents and an
uncapped answer is unreadable rather than thorough.

### Phase two, designed and deliberately not built

Searching by meaning belongs in a **separate binary in this repository**, not in `pubkit`:

- `pubkit` stays dependency-light and starts in milliseconds; an embedding model does not.
- The index is a **sidecar** beside the library (`<library>/semantic.<lang>.idx`), never inside
  `jwlib*.db`, so it can be deleted, rebuilt or ignored without touching what is authoritative.
- A small CPU model, quantised, over paragraphs already in `par` — the text never leaves the
  machine, which is the same constraint everything else here works under.
- It answers only what this phase cannot: a question sharing no vocabulary with its answer. The
  ranking still has to put prose first and name its evidence, and where the two disagree the
  deterministic answer is the one that can be checked.

What makes that buildable now is that this phase gave it its inputs: `Question` already separates
signal words from noise, `documentKind` already knows what a document is for, and every hit
already carries a citation and an address.

## A library holds several Bibles, and reads name the one they want

A study edition and a plain edition carry the same BibleVerseId for every verse, and the
glossary of terms only ships with the plain one — so holding both is the ordinary case. With
the verse id as the primary key, indexing the second Bible silently took over the first one's
verses: measured on a live library, where a study edition became a plain one without a word,
losing the study notes' companion text.

Verses are therefore keyed per publication, and every read names its Bible. Which one? The one
that can answer most, which is the one with study notes; failing that, the one with the most
verses. It is cached per handle because it is asked on every call.

That change touches one table, so it migrates on its own (`migrateVerses`) rather than through
the schema version: bumping the version drops every indexed publication and would have cost a
full re-sync of a 1.3 GB library for a change to one table. The Bible comes back with the next
sync, which reads the cached `.jwpub` and needs no network.

## One connection means a query inside a row loop deadlocks

The pool is deliberately one connection (SQLite, single writer). Resolving a glossary term
while iterating the study-note rows therefore asks for a connection the loop is itself holding,
and waits forever. It does not look like a deadlock from outside — it looks like a hang, and it
survived four rounds of guessing at slow SQL before a goroutine dump named the line in seconds.

The links are read during the loop and resolved after the rows are closed. The fixtures that
would have caught it all left the note HTML empty, so nothing exercised the read at all; there
is now a test with a real note body in it.

## Study notes point at a dictionary by document id, so the pointer is followed, not read

A note that says "see Glossary, X" expresses it as `<a class="xt" href="jwpub://p/<LANG>:<id>/">`.
The class name and the scheme are untranslated and the id is the same in every language, so the
reference is resolved by following the link. The entries themselves are published online rather
than inside any JWPUB, so what is given is the identifier and the address — correct everywhere —
rather than a definition the library does not hold.

A Bible in the library carries a glossary of its own (`Document.Class` 116, verified identical
in Spanish and Japanese; the entries are marked by the untranslated `de`/`dt`/`dd` classes). It
is indexed at sync time, so where it defines the term the reader gets the meaning offline. It
does not cover every term the dictionary has, and a term it cannot define shows the address
rather than something adjacent: an empty definition means "not held", never "no such term".

## Addresses use jw.org's finder, not a wol path

Every link this tool printed was hardcoded to one language: `wol.jw.org/es/wol/d/r4/lp-s/…`.
A wol path embeds a per-language repository id (`r4`) and library code (`lp-s`) that cannot be
derived from anything inside a JWPUB, so there was no way to build the Spanish form for an
arbitrary language.

`https://www.jw.org/finder?docid=<id>&par=<n>&wtlocale=<lang>` needs only what is already
known — the MEPS document id and the language symbol — resolves to the article in that
language, and lands on the paragraph anchor. Verified in Spanish and English against the live
site.

## Citations quote the publisher where the publisher speaks

A citation composed from parts is a guess about someone else's conventions. Where a publication
states its own — the caption of an extract is written by the publication, in its own language —
that string is passed through verbatim. Only where there is none is one assembled, and then in
the shortened form the publications themselves adopted (`w24.05`), with the pre-2016 dated form
kept as published (`w13 15/1`).

The page number is missing from assembled citations because it is not recorded: page breaks are
in the document markup and the index does not keep them. That is a gap to close at index time,
not something to approximate.

## Reading-aloud time is counted per script, and says it is an estimate

Counting words to estimate reading time is wrong for scripts written without spaces: a Chinese
paragraph would come out as a couple of seconds. Words are counted where the script separates
them and characters where it does not, each at its own rate, decided by the text rather than by
a language setting so a quotation in another script is still counted sensibly. The rates are
assumptions, labeled as such in the output and overridable.

## Removing a library is a command, because doing it by hand is a trap

SQLite in WAL mode keeps recently written content in the `-wal` file beside the database. A
library that has just indexed a Bible can therefore show a 4 KB `.db` next to a 46 MB `-wal`:
`ls -l` makes a full library look empty. That is not a hypothetical — a 780-document Bible was
deleted on exactly that reasoning, and removing only the `.db` left the `-wal` behind to be
replayed onto the next database created in its place.

So `pubkit drop` exists: it counts the publications, names the three files, reports without
`--yes`, closes the handle before touching anything, and removes the database with its `-wal`
and `-shm` together. The shared `.jwpub` cache is kept unless `--cache` asks for it, since
re-indexing from it needs no network.

`Store.Size()` counts all three files and `pubkit config` prints the database path, because
half of that mistake was reasoning about a file the tool never showed anyone.

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
