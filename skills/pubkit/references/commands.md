# Command surface and --json

Every command accepts `--json`, `--language`, `--library` and `--offline`. Run
`pubkit <command> --help` for the full flags.

## Fetch
- `pubkit sync <symbol>... [--issue AAAAMM] [--plan] [--file path.jwpub]` downloads, verifies
  the published checksum, decrypts and indexes. A copy that already matches is not re-downloaded.
  `--plan` changes nothing and reports download size and estimated disk use.
- Useful symbols: `mwb` (meeting workbook, with `--issue`), `w` (Watchtower study, `--issue`),
  `nwtsty` (study Bible), `it`, `wcg`, `lmd`, `th`.

## Read (all offline once synced)
| Command | JSON top-level keys |
|---|---|
| `week <date>` | `monday`, `range`, `workbook`, `songs`, `weekly_reading`, `student_reading`, `sections`, `images`, `videos`, `watchtower` |
| `verse "<ref>"` | `reference`, `translation`, `verses`, `ranges`, `cited_in`, `commented_in`, `citing_documents_total` |
| `search "<words>"` | `query`, `documents`, `verses`, `videos`, `fts` |
| `doc <docid>` | `docid`, `key`, `publication`, `title`, `url`, `paragraphs`, `bible_citations`, `references`, `images`, `videos` |

Other readers: `chain`, `graph` (every edge names the table it came from; nothing is
inferred), `dossier`, `watchtower`, `daily`, `image`, `places`, `reading-time`, `pubs`.

## Video
`subtitles <key>` fetches and prints a transcript (`--format vtt` for WebVTT); fetching adds it
to the set `subtitles find "<phrase>"` searches. `media clip` and `media frame` need `ffmpeg`
and the network.

## Network use
`sync`, `catalog`, `update-week`, `subtitles` retrieval and `media clip|frame` need jw.org.
`week` fetches a missing Watchtower study issue unless `--offline` is given.

## Backups
`backup inspect|merge|sync|status|annotate` work on private JW Library `.jwlibrary` backups.
Restoring a backup in JW Library replaces the device's personal study data. Never overwrite or
send a user's backup anywhere; see `docs/backup.md` in the repository.
