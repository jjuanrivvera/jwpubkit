# Changelog

## v0.12.0 - 2026-10-08

- Add `backup inspect`, `backup merge` and `backup annotate` for JW Library `.jwlibrary` backups (aliases `respaldo`,
  `inspeccionar`, `unir`, `anotar`). Merge two or more backups, deduplicating locations by natural key, notes and
  highlights by GUID and tags by type and name, remapping every ID and carrying media files.
- Choose conflict winners with `--prefer <file|newest|oldest>`, per table when needed; review conflicts one by one with
  `--interactive` (word diff, keep A or B, concatenate, edit in `$EDITOR`, skip) or report without writing (`--dry-run`).
- Propagate deletions with three-way merges against a common ancestor (`--base`), including delete/edit conflicts,
  re-creation by GUID and dependent rows.
- Maintain a master backup with `backup sync` and `backup status`: store folder from `backup_store` in the config file,
  the environment or `--store`, atomic master and base, rotating history and optional `--watch` of an inbox folder.
- `backup annotate` adds highlights, notes and text-field answers anchored with undated publication keys.
- Exercise every merge path with invented schema-16 databases; no real backups in the repository. Raise the coverage
  floor to 70% (measured total about 72%).
- Build with Go 1.26.9 and golang.org/x/net v0.60.0, fixing GO-2026-6617 in net/http.

## v0.11.0 - 2026-09-30

- Capture JPG/PNG video frames at repeated marks or regular intervals, with optional
  timestamped contact sheets, rendition selection and shared traffic/output budgets.
- Cache bounded 256 KiB ranges between frame seeks and pace uncached origin requests;
  validate staged images and preserve existing paths on failed batches.
- Add silent MP4 clips (`--no-audio`) and audio-only M4A clips (`--audio-only`).
- Exercise frame timing, range reuse, output rollback and audio flags with invented
  images and local HTTP/tool doubles. Reject non-finite and overflowing clock times.
- Raise the matching local and CI coverage floor to 66% (measured total about 68.5%).

## v0.10.1 - 2026-09-30

- Resolve historical public Watchtower references from 2008–2015 under the wp API symbol,
  even when their JWPUB reference metadata still uses w. Study and earlier issues keep w.
- Raise the local and CI coverage floor to 65%, backed by the synthetic test suite.

## v0.10.0 - 2026-09-30

- Discover audiovisual metadata by traversing all mediator categories, deduplicating keys
  and preserving provenance, file sizes, checksums and official subtitle URLs.
- Sync catalog subtitles with checksum-aware resume, request pauses and response-body byte
  budgets. Record pending, indexed, unavailable and failed states without downloading videos;
  optionally use pub-media to find subtitle URLs missing from mediator.
- Search catalog transcripts across adjacent cues and return surrounding context, timestamps,
  category, date and provenance.
- Cut remote MP4 fragments in copy or exact mode through a range-only proxy with separate
  traffic and output budgets, cancellation and effective-duration reporting.
- Extract images from the multimedia table as well as document markup, including SVG figures
  and local image dependencies. Select by paragraph, cited passage, filename or file genre.
- Plan publication sync without library writes; report download and disk estimates, enforce
  planned byte budgets and pause between publications. List publication formats by symbol,
  year and issue, including historical half-month issues.
- Resolve missing workbook references during weekly updates. Accept Bible ranges in places
  and classify source evidence without claiming inferred locations on maps.
- Add offline tests using synthetic publications, subtitle files and local HTTP doubles.
