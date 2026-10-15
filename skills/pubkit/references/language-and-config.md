# Language, library and settings

## Language
Publications are fetched and read in English (`E`) unless told otherwise. Pass `--language S`
(jw.org symbols: `E` English, `S` Spanish, `F` French...) or set it once in
`~/.config/pubkit/config`:

```ini
language = S
library = ~/.local/share/jwlib
media_dir = ~/media
```

Each language has its own database file inside the library folder. `pubkit drop --language E`
reports what it would remove and removes nothing without `--yes`.

## Precedence
Flag, then environment, then the settings file, then the default.

| Setting | Flag | Environment | File key | Default |
|---|---|---|---|---|
| Language | `--language` | `JWPUBKIT_LANG` | `language` | `E` |
| Library folder | `--library` | `JWPUBKIT_HOME` | `library` | `~/.local/share/jwlib` |
| Media folder | | `JWPUBKIT_MEDIA_DIR` | `media_dir` | `media/` in the library |
| Backup store | `--store` | `JWPUBKIT_BACKUP_STORE` | `backup_store` | `backups/` in the library |

`pubkit config` prints every setting and what decided it. Run it before guessing why a command
behaved oddly: an exported variable in a shell profile never reaches cron jobs or services, and
one set on this machine will silently override the file.

## Bible references
References parse in any known language and print in the language of the Bible that was synced.
English and Spanish are built in; other languages are learned from the synced Bible.
