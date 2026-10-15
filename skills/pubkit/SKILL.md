---
name: pubkit
description: Query a local library of JW publications (JWPUB) with the `pubkit` CLI — the week's meeting, Bible passages with notes and cross references, full-text search, documents, images and video subtitles, all offline after a sync. Use when the user wants to prepare a meeting, look up a scripture or what a publication says, search publications, or read a video transcript. Triggers: "pubkit", "jwpub", "this week's meeting", "workbook", "watchtower study", "look up this scripture", "search the publications", "video transcript".
version: 0.1.0
homepage: https://github.com/jjuanrivvera/jwpubkit
license: MIT
allowed-tools: Bash(pubkit:*)
metadata: {"openclaw":{"category":"research","emoji":"📖","requires":{"bins":["pubkit"]}}}
---

# pubkit — a local library of JW publications

`pubkit` downloads publications in JWPUB format from the open jw.org CDN, decrypts them and
indexes them into a local SQLite library. Every query after the sync is local and
deterministic: use it to fetch the text, then do the thinking yourself. Do not recall
publication text from memory when the library can return it.

## Prerequisites

- `pubkit` on the `PATH` (`pubkit version`). Releases for Linux, macOS and Windows:
  https://github.com/jjuanrivvera/jwpubkit/releases. If it is missing, tell the user how to
  install it; do not download or build it unasked.
- Publications synced into the library (`pubkit pubs` shows what is there).
- No account or token is needed.

## Golden rules

- **Look before you fetch.** `pubkit config` (language, library folder, what decided each) and
  `pubkit pubs` (what is indexed).
- **Ask before downloading.** `sync` needs the network and large publications are big (the
  study Bible is ~128 MB). Use `pubkit sync <symbol> --plan` first and tell the user the size.
  When a command reports a missing publication it prints the exact `pubkit sync ...` line.
- **Use `--offline`** when you must guarantee no network. **Use `--json`** when you will
  process the output.
- **Quote, don't paraphrase as quotation.** Give the publication and paragraph text came from;
  never present your own wording as publication text.
- **Personal use only.** Do not republish extracted text, images or transcripts, and do not
  commit them to a repository.

## Workflow (check -> sync -> query)

```sh
pubkit config                             # language and library in effect
pubkit sync mwb --issue 202609            # meeting workbook, Sept-Oct 2026
pubkit week 2026-09-28                    # the week's meeting, assembled
pubkit verse "John 3:16" --json           # needs the Bible: pubkit sync nwtsty
```

## Command cheatsheet

| Goal | Command |
|---|---|
| What the library holds | `pubkit pubs` |
| Fetch and index a publication | `pubkit sync <symbol> [--issue AAAAMM] [--plan]` |
| The week's meeting | `pubkit week 2026-09-28 [--offline]` |
| The week's study article | `pubkit watchtower 2026-09-28` |
| A passage with notes and references | `pubkit verse "John 3:16"` |
| Follow marginal references | `pubkit chain "Jer 38:6" --hops 2` |
| What a passage connects to | `pubkit graph "Jer 38:6"` |
| Everything around a passage | `pubkit dossier "Jer 38:1-13"` |
| Search all text | `pubkit search "cistern" --limit 5` (`--pub it,w`, `--bible`, `--videos`, `--all-words`) |
| A document by id | `pubkit doc 202026255` (`--format json\|text`) |
| A document's images | `pubkit image 202026255 --list` |
| Transcribe a video | `pubkit subtitles pub-jwb-125_4_VIDEO` |
| Find a phrase in fetched transcripts | `pubkit subtitles find "end of the day"` |
| Reading time aloud | `pubkit reading-time --reference "Jer 38:1-13"` |
| Another language | add `--language S` (jw.org symbols: `E`, `S`, `F`...) |

## Troubleshooting

- Result in the wrong language -> `pubkit config`; an exported `JWPUBKIT_LANG` overrides the
  settings file, and a flag overrides both.
- "No transcripts have been fetched yet" -> `pubkit subtitles <key>` first; `find` only searches
  what you already fetched.
- A 404 on `sync` -> that issue is not published yet; check the issue number.
- An empty search result does not prove a passage or video does not exist.

See `references/` for the language and settings rules, the command surface with `--json` keys,
and limits.
