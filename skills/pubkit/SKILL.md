---
name: pubkit
description: Query a local library of JW publications (JWPUB) with the pubkit CLI - the week's meeting, Bible passages with notes and cross references, full-text search, documents, images and video subtitles. Use when the user asks to prepare a meeting, look up a scripture or what a publication says, search publications offline, or read a video transcript, and pubkit is installed.
---

# pubkit

`pubkit` downloads publications in JWPUB format from the open jw.org CDN, decrypts them and
indexes them into a local SQLite library. Every query after the sync is local and deterministic,
so use it to fetch the text, then do the thinking yourself. Do not recall publication text from
memory when the library can return it.

## When to use it

- "What is the meeting this week?" / "Prepare the week of 2026-09-28" -> `week`
- "Read John 3:16 with its notes" / "What does this passage cross-reference?" -> `verse`, `chain`, `graph`
- "Where do the publications talk about X?" -> `search`
- "Show me document 202026255" / "its images" -> `doc`, `image`
- "What is said in this video?" -> `subtitles`

Do not use it for anything that needs a publication the user has not synced, unless the user
agrees to a download.

## Before you start

```sh
pubkit version
pubkit config        # language, library folder and what decided each
pubkit pubs          # what is already in the library
```

If `pubkit` is not on the `PATH`, stop and tell the user how to install it (see the project
README). Do not build or download it yourself without being asked.

## Language and library

Publications are fetched in English (`E`) unless told otherwise. Pass `--language S` (jw.org
symbols: `E`, `S`, `F`...) or rely on `language = S` in `~/.config/pubkit/config`. Precedence:
flag, then environment (`JWPUBKIT_LANG`, `JWPUBKIT_HOME`), then the file, then the default.
Each language has its own database. Check `pubkit config` when a result looks like the wrong
language, because an exported variable can override the file.

## Commands

Add `--json` to any command for structured output. Use `--offline` to guarantee no network.

```sh
# Fetch (needs the network). Skips the download when the local copy matches the checksum.
pubkit sync mwb --issue 202609            # meeting workbook, Sept-Oct 2026
pubkit sync nwtsty                        # Bible, study edition (~128 MB)
pubkit sync it --plan                     # what a sync would cost, changes nothing

# The week (fetches the Watchtower study issue if missing, unless --offline)
pubkit week 2026-09-28
pubkit week 2026-09-28 --offline --json   # keys: workbook, songs, sections, images, videos, watchtower...

# Passages
pubkit verse "John 3:16" --json           # keys: reference, verses, ranges, cited_in, commented_in
pubkit chain "Jer 38:6" --hops 2          # marginal references, each verse with its text
pubkit graph "Jer 38:6"                   # who cites it, what it cites; every edge names its source table
pubkit dossier "Jer 38:1-13"              # everything around a passage in one document

# Search and documents
pubkit search "cistern" --limit 5         # --pub it,w  --bible  --videos  --all-words
pubkit doc 202026255                      # Markdown by default; --format json|text
pubkit image 202026255 --list             # --list never writes files or uses the network
pubkit watchtower 2026-09-28              # the week's study article with questions

# Video
pubkit subtitles pub-jwb-125_4_VIDEO      # transcript; fetching adds it to the searchable set
pubkit subtitles find "end of the day"    # only searches transcripts already fetched
pubkit reading-time --reference "Jer 38:1-13"
```

A search or week result prints a `docid`; pass it to `doc` or `image`. Hits carry a jw.org link
that opens the paragraph. If a command reports a missing publication, it prints the exact
`pubkit sync ...` line to run; run it only if the user agrees to the download.

## Working rules

- Quote text as the publication wrote it and say which publication and paragraph it came from.
  Never present your own wording as publication text.
- Prefer `--json` when you will process the output, and plain output when you will show it.
- `sync` of large publications (a Bible, `it`) takes time and disk; use `--plan` first and tell
  the user the size.
- Sync one thing at a time. Two concurrent `sync` runs of the same publication can collide.

## Limits and personal use

- Nothing is bundled: results come only from what the user synced on their own machine.
- `sync`, `catalog` and subtitle retrieval need jw.org to be reachable.
- JWPUB layouts differ between publication generations; unsupported ones return fewer fields.
- A missing search hit does not prove a passage or video does not exist.
- `backup *` works on private JW Library backups and `backup annotate` is experimental. Never
  restore or overwrite a user's backup, and never send backup data anywhere.
- Publication content belongs to its owner. The tool is for the user's own study: do not
  republish extracted text, images or transcripts, and do not commit them to a repository.
