# Backups

Moved out of the README unchanged. See the [README](../README.md) for installation and the basics.


`backup` works locally with schema 16 `.jwlibrary` user-data backups. It validates the
manifest hash, SQLite integrity, foreign keys and referenced media. No backup data is
sent to a service. Restoring a backup in JW Library **replaces the device's personal
study data**: export a fresh backup from every device before restoring anything.

```sh
pubkit backup inspect first.jwlibrary
pubkit backup merge first.jwlibrary second.jwlibrary third.jwlibrary \
  --prefer newest --device-name "Combined library" -o combined.jwlibrary
pubkit backup merge first.jwlibrary second.jwlibrary --prefer first.jwlibrary \
  --prefer-notes newest --prefer-input-fields oldest --dry-run
pubkit backup merge first.jwlibrary second.jwlibrary --interactive \
  --report merge-report.json -o reviewed.jwlibrary
```

The first input wins by default. `--prefer` accepts any input path, `newest` or
`oldest`; dates come from the archive's `lastModifiedDate`, and equal dates retain
input order. The same choices work for `--prefer-notes`, `--prefer-highlights`,
`--prefer-input-fields`, `--prefer-tags` and `--prefer-bookmarks`. Tags are identified
by `(Type, Name)`, so differently named tags coexist. Location keys, GUIDs, all
references and playlist relationships are remapped. Identical playlist content and
relationships are deduplicated across inputs; different items remain separate.
Preferred tag membership positions are retained; additional memberships append in order.
Colliding media filenames are renamed and references follow the new path.

Different-device highlight overlaps are reported. The preferred range keeps its
color and only uncovered numeric tokens are imported. Token boundaries are
inclusive. If a whole-block range has unknown bounds, the overlapping preferred
range wins for that block and the loss is reported. Existing overlaps within one
input are preserved. `--dry-run` performs the merge and validation in temporary
storage without writing an output archive. Inputs and existing outputs are never
overwritten. The JSON report lists counts, identifiers, differing fields and
overlaps, without note bodies or answer values; treat its identifiers as private.

`--interactive` requires a terminal and reviews conflicting note titles/bodies and
textarea answers one by one. It shows both versions and a word diff, with choices
to keep A or B, concatenate with a separator, edit with `$EDITOR`, or skip the
decision and retain `--prefer`. Metadata-only conflicts use the configured strategy.
The editor receives a temporary private file, deleted afterwards. For unattended
merges, use `--prefer`; requesting interaction without a TTY fails clearly.

With a common ancestor, deletion-aware merges use exactly two descendants:

```sh
pubkit backup merge local.jwlibrary incoming.jwlibrary --base ancestor.jwlibrary \
  --prefer incoming.jwlibrary --dry-run --report deletions.json
pubkit backup merge local.jwlibrary incoming.jwlibrary --base ancestor.jwlibrary \
  --prefer newest --interactive -o next.jwlibrary
```

An ancestor row missing from either side is deleted if the other side left it
unchanged. Deletion versus editing is a conflict and uses the global or table
preference. In a three-way merge, a one-sided edit also survives an unchanged
preferred side. New rows absent from the ancestor are additions. Notes use GUIDs;
a recreated note with the same GUID and changed creation time or content counts
as an edit. A byte-identical recreation cannot be distinguished from the original.
Highlights include their complete block-range set. Deleting a highlight detaches
surviving notes. Deleting a note, tag or playlist cascades to its relationships;
keeping an edited parent restores children removed by the opposing cascade.
`--prefer-playlists` and `--prefer-tag-maps` control those conflicts. Interactive
three-way review offers A, B or skip for deletion and structural conflicts.
The report includes the ancestor counts, deletion attempts, their outcomes and
counts of removed ancestor rows, including in `--dry-run`.

Tags, bookmarks, playlist items and playlist markers have no GUID. Unchanged rows are matched
by their fields and relationships use remapped identities; edited rows fall back
to their ancestor IDs. Three-way output preserves those ancestor IDs. Use descendants of the supplied ancestor: independently
reassigned IDs on edited GUID-less rows cannot reliably identify their lineage.
Without `--base`, merge continues to union any number of inputs without propagating
deletions. Use the correct ancestor rather than an arbitrary older backup.

### Keeping a master backup

Set a dedicated private store with the existing configuration format:

```ini
# ~/.config/pubkit/config
backup_store = ~/backups/study
```

`JWPUBKIT_BACKUP_STORE` overrides the file (`JWLIB_BACKUP_STORE` is also accepted),
and `--store` overrides both. Without a setting, the store is `backups/` under the
selected library directory. `pubkit config` shows `backup_store` and its origin.

```sh
pubkit config --json
pubkit backup sync initial.jwlibrary --device-name "Study master"
pubkit backup sync workstation.jwlibrary tablet.jwlibrary --prefer newest
pubkit backup status
pubkit backup status --json --store ~/backups/study
pubkit backup sync --watch ~/incoming-backups --history-limit 10
```

The first import initializes the master. If the first batch contains several
unrelated backups, they are unioned because no ancestor is known yet. Subsequent
batches merge the master with every incoming backup against **one common stored
ancestor** and propagate deletions. The ready-to-restore filename is always
`<store>/master.jwlibrary`. After a successful batch, `<store>/base.jwlibrary` is
also updated to that result. Export backups from devices that restored that
master before the next batch. Collect descendants of one distributed master into
the same invocation; a snapshot from an older branch requires the appropriate
ancestor with `--base <older-master.jwlibrary>`, available in the history reported
by `backup status`. A single stored base cannot infer arbitrary device ancestry.
This also applies to consecutive batches in watch mode.

`--prefer` defaults to `newest`. It accepts `newest`, `oldest`, `master`, `incoming`
or an incoming path. Table preferences and `--interactive` work as in `backup
merge`. Original input dates decide conflicts throughout a batch; an intermediate
save date never gives a side extra priority. `--device-name` defaults to `pubkit
master`. To preview a synchronization, use `backup merge` with the current master,
incoming file and `--base <store>/base.jwlibrary --dry-run --report preview.json`.

The store keeps complete versions under `history/`, with `current.json` selecting
one version containing its master, base and bookkeeping. Stable archive names
export that version. A process lock prevents concurrent updates, and the pointer
changes only after the whole batch validates and is saved. Failed batches leave
the previous master intact. `backup status` repairs exports after an interrupted
publication and shows the master, archive date, device name, table counts, last
synchronization and retained history (history paths are included with `--json`).
The default rotation retains ten previous masters plus the current one;
`--history-limit 0` keeps only the current version. Incoming snapshots are tracked
by file hash, so repeating an already imported file does not apply it again.
Ordinary `sync` keeps incoming files in place.

`--watch <directory>` polls until interrupted, waits for two unchanged scans,
validates complete `.jwlibrary` files and imports each ready batch. Successfully
imported files move to `<directory>/procesados`; filename collisions preserve both
files. Invalid or incomplete files stay in place and are retried when they change.
Temporary store failures retry automatically. Import bookkeeping makes a retry
safe if the process stopped between updating the master and moving an input.
The incoming directory and store must be separate. Watch emits JSON events,
including errors; `procesados` retains original incoming files independently of
master history rotation.

Spanish aliases include `respaldo sincronizar`, `respaldo estado`, `--almacen`,
`--vigilar`, `--historial`, `--ancestro` and `--informe`; the help and output remain
in English. Existing aliases such as `--preferir`, `--interactivo` and
`--nombre-dispositivo` also apply.

`backup annotate` is an experimental prototype for publication paragraphs and
workbook textareas, using decrypted HTML already indexed in the local library:

```sh
pubkit backup annotate first.jwlibrary --language E --plan annotations.json \
  --device-name "Annotation experiment" -o experiment.jwlibrary
```

An invented plan illustrates the format; replace its document id, paragraph id and
quote with values from your own library:

```json
{
  "highlights": [{"docid": 123456, "pid": 4, "quote": "purple robots", "color": 1}],
  "note": {"highlight": 0, "title": "Invented observation", "content": "The robots are imaginary."},
  "answers": [{"docid": 123456, "text_tag": "tt11", "value": "An invented answer."}]
}
```

The note's `highlight` is a zero-based index into the plan. Colors range from 1 to
6. Paragraph `pid` comes from HTML `data-pid`, which differs from the printed
paragraph number. Answers require an actual `textarea` id, not just any `tt<n>`
element. Existing highlights and answers are never replaced. Tokenization is an
unofficial reconstruction that counts Unicode words and punctuation; inspect the
result visually in JW Library before relying on it. Bible verse annotation and
exact tokenizer parity across languages and publication editions remain unverified.

Spanish aliases are `respaldo`, `inspeccionar`, `unir` and `anotar`. Compatible flag
aliases include `--salida`, `--preferir`, `--simular`, `--interactivo` and
`--nombre-dispositivo`.

