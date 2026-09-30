// Package store is the local library: cached JWPUB files plus a SQLite
// database with the decrypted text, paragraph FTS5 index, Bible verses, study
// notes, citations, media and extracts of every synced publication.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
)

// SchemaVersion changes when the tables change; a mismatch rebuilds the
// database (the JWPUB cache survives, so re-indexing needs no download).
const SchemaVersion = "3"

// Store is an open library.
type Store struct {
	DB  *sql.DB
	ctx context.Context
	// Dir is the library directory, shared by every language.
	Dir string
	// Lang is the language this handle holds; Path is the file it holds it in.
	Lang string
	Path string

	// bibleID caches which of the library's Bibles verses are read from.
	bibleID int64

	// Progress, when set, is told about work that takes long enough to notice.
	Progress func(format string, args ...any)
}

// DefaultDir is $JWPUBKIT_HOME, else $JWLIB_HOME, else $XDG_DATA_HOME/jwlib, else
// ~/.local/share/jwlib.
func DefaultDir() string {
	// JWPUBKIT_HOME is the current name; JWLIB_HOME keeps working because libraries built
	// before the rename live where it points, and moving someone's 900 MB library to match a
	// new binary name would be a rude way to ship a rename.
	if d := os.Getenv("JWPUBKIT_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("JWLIB_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "jwlib")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "jwlib")
}

// PubsDir holds the downloaded .jwpub files.
func (s *Store) PubsDir() string { return filepath.Join(s.Dir, "pubs") }

// legacyDB is the single-language database the library used before it could hold
// more than one language. It keeps serving whichever language it already holds.
const legacyDB = "jwlib.db"

// dbFile picks the database for a language. Each language gets its own file
// because a MEPS document id is THE SAME NUMBER in every language: one shared
// table would have every edition of a publication overwriting the previous one,
// and no amount of remembering to filter by language in each query would make
// that safe. Separate files make the collision impossible instead of forbidden.
func dbFile(dir, lang string) string {
	legacy := filepath.Join(dir, legacyDB)
	if lang == "" {
		return legacy
	}
	switch held, err := languageOf(legacy); {
	case err != nil: // no legacy database: this language starts its own
	case held == "": // an empty legacy database is claimed by whoever opens it first
		return legacy
	case held == lang:
		return legacy
	}
	return filepath.Join(dir, "jwlib."+lang+".db")
}

// languageOf reports which language an existing database holds, or "" when it
// holds nothing yet. It returns an error when there is no database at all.
func languageOf(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var lang string
	// More than one language in one file can only be a library written before
	// they were separated; the first one wins, and the rest move to their own.
	if err := db.QueryRow(`SELECT lang FROM pub GROUP BY lang ORDER BY count(*) DESC LIMIT 1`).Scan(&lang); err != nil {
		return "", nil
	}
	return lang, nil
}

// Open opens (and creates or migrates) the library in dir for one language.
func Open(dir, lang string) (*Store, error) { return OpenWithProgress(dir, lang, nil) }

// OpenWithProgress is Open with somewhere to report one-off work that takes long
// enough for a user to wonder whether anything is happening.
func OpenWithProgress(dir, lang string, progress func(string, ...any)) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "pubs"), 0o755); err != nil {
		return nil, err
	}
	path := dbFile(dir, lang)
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=temp_store(MEMORY)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Dir: dir, Lang: lang, Path: path, Progress: progress}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	s.teachBookNames()
	if err := s.backfillDocVideos(); err != nil {
		return nil, err
	}
	return s, nil
}

// backfillDocVideos fills the document-to-video table from the HTML the library
// already stores, once.
//
// The alternative was asking for a re-sync of every publication to record an edge
// that is already on disk — over a gigabyte of downloads for a table that can be
// derived from what is here. It runs once, marks itself done, and says so while it
// works, because a first command that takes half a minute with no explanation is
// indistinguishable from a hang.
func (s *Store) backfillDocVideos() error {
	var done string
	if err := s.DB.QueryRow(`SELECT value FROM meta WHERE key='doc_video_backfill'`).Scan(&done); err == nil && done == "1" {
		return nil
	}
	var docs int
	if err := s.DB.QueryRow(`SELECT count(*) FROM doc WHERE html <> ''`).Scan(&docs); err != nil || docs == 0 {
		// Nothing indexed yet: the table will fill as publications are synced.
		_, err := s.DB.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('doc_video_backfill','1')`)
		return err
	}
	if s.Progress != nil {
		s.Progress("recording which documents embed which video, once (%d documents)", docs)
	}
	rows, err := s.DB.Query(`SELECT docid, pub_id, html FROM doc WHERE html <> ''`)
	if err != nil {
		return err
	}
	type found struct {
		docid, pubID int
		keys         []string
	}
	var all []found
	for rows.Next() {
		var docid, pubID int
		var html string
		if err := rows.Scan(&docid, &pubID, &html); err != nil {
			rows.Close()
			return err
		}
		if keys := parsedVideos(html); len(keys) > 0 {
			all = append(all, found{docid, pubID, keys})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only when Commit did not run
	ins, err := tx.Prepare(`INSERT OR REPLACE INTO doc_video(docid, pub_id, key) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, f := range all {
		for _, k := range f.keys {
			if _, err := ins.Exec(f.docid, f.pubID, k); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('doc_video_backfill','1')`); err != nil {
		return err
	}
	return tx.Commit()
}

// addColumns adds columns a table is missing, leaving its rows alone. The values
// arrive with the next sync of the publication that supplies them.
func (s *Store) addColumns(table string, cols map[string]string) error {
	rows, err := s.DB.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(have) == 0 {
		return nil // the table does not exist yet; the schema will create it
	}
	for col, typ := range cols {
		if have[col] {
			continue
		}
		// #nosec G202 -- the names are constants in this file, never user input
		if _, err := s.DB.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` ` + typ); err != nil {
			return err
		}
	}
	return nil
}

// migrateVerses rebuilds the verse table when it still uses the old layout, in
// which the BibleVerseId was the primary key and a second Bible therefore
// overwrote the first. Nothing is lost that cannot be rebuilt: the verses come
// back with the next sync of a Bible, which reads the cached .jwpub and needs
// no network.
func (s *Store) migrateVerses() error {
	rows, err := s.DB.Query(`PRAGMA table_info(verse)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	existed, keyed := false, false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		existed = true
		if name == "row_id" {
			keyed = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if keyed {
		return nil
	}
	if existed {
		for _, q := range []string{
			`DROP TRIGGER IF EXISTS verse_ai`, `DROP TRIGGER IF EXISTS verse_ad`,
			`DROP TABLE IF EXISTS verse_fts`, `DROP TABLE IF EXISTS verse`,
		} {
			if _, err := s.DB.Exec(q); err != nil {
				return err
			}
		}
	}
	_, err = s.DB.Exec(verseSchema)
	return err
}

// teachBookNames hands the bible package every language this library has seen a
// Bible in, so references parse and print in the language of the publications
// actually on disk. A library with no Bible simply leaves the built-in names.
func (s *Store) teachBookNames() {
	rows, err := s.DB.Query(`SELECT lang, book, name FROM book_name`)
	if err != nil {
		return
	}
	defer rows.Close()
	byLang := map[string]map[int]string{}
	for rows.Next() {
		var lang, name string
		var book int
		if err := rows.Scan(&lang, &book, &name); err != nil {
			return
		}
		if byLang[lang] == nil {
			byLang[lang] = map[int]string{}
		}
		byLang[lang][book] = name
	}
	for lang, names := range byLang {
		bible.Register(lang, names)
	}
}

// Files are the files that make up this language's database. SQLite in WAL mode
// keeps recent content in the -wal file, so the three are one unit: the main file
// can be a few kilobytes while megabytes of indexed publications sit beside it,
// and deleting one of the three leaves the rest to be applied to whatever is
// created next. Anything that removes a library removes these together.
func (s *Store) Files() []string {
	return []string{s.Path, s.Path + "-wal", s.Path + "-shm"}
}

// Size is how much disk this language's database takes, counting the -wal, which
// is where the bytes actually are right after indexing.
func (s *Store) Size() int64 {
	var total int64
	for _, f := range s.Files() {
		if st, err := os.Stat(f); err == nil {
			total += st.Size()
		}
	}
	return total
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		return err
	}
	// Two columns added to verse_note after libraries existed: adding a column is
	// cheap and keeps the indexed content, unlike a schema bump.
	if err := s.addColumns("verse_note", map[string]string{
		"begin_pid": "INTEGER", "end_pid": "INTEGER",
	}); err != nil {
		return err
	}
	// A Bible that predates verses being keyed per publication has to be rebuilt,
	// but only it: bumping the schema version would drop every indexed
	// publication in the library and cost the user a full re-sync for a change
	// that touches one table.
	if err := s.migrateVerses(); err != nil {
		return err
	}
	// Tables that can simply be added never justify a rebuild: a schema bump drops
	// the indexed content, and re-indexing a full library costs minutes the user
	// did not ask for. They are created on every open and fill up on the next sync.
	if _, err := s.DB.Exec(additiveSchema); err != nil {
		return err
	}
	var v string
	err := s.DB.QueryRow(`SELECT value FROM meta WHERE key='schema'`).Scan(&v)
	if err == nil && v == SchemaVersion {
		return nil
	}
	if err == nil && v != SchemaVersion {
		// Older layout: drop everything except the video cache, re-index later.
		for _, t := range []string{"pub", "doc", "par", "par_fts", "verse", "verse_fts", "verse_note",
			"verse_fn", "verse_xref", "cite", "media", "extract", "dated", "question", "refpub"} {
			if _, err := s.DB.Exec("DROP TABLE IF EXISTS " + t); err != nil {
				return err
			}
		}
	}
	if _, err := s.DB.Exec(schema); err != nil {
		return fmt.Errorf("creating the schema: %w", err)
	}
	_, err = s.DB.Exec(`INSERT OR REPLACE INTO meta(key, value) VALUES('schema', ?)`, SchemaVersion)
	return err
}

// additiveSchema holds tables that are safe to create on an existing library.
const additiveSchema = `
-- Which documents embed which video. The link lives in the document markup, so
-- without recording it the only way to answer "where is this video used" would be
-- to re-parse every document in the library.
CREATE TABLE IF NOT EXISTS doc_video(
	docid INTEGER NOT NULL, pub_id INTEGER NOT NULL, key TEXT NOT NULL,
	PRIMARY KEY(docid, key)
);
CREATE INDEX IF NOT EXISTS doc_video_key ON doc_video(key);

-- The entries of any glossary the library holds, so a study note's "see
-- Glossary, X" can be answered offline when the publication carrying it is
-- synced. The term is stored folded for lookup and as published for display.
CREATE TABLE IF NOT EXISTS glossary(
	pub_id INTEGER NOT NULL, docid INTEGER NOT NULL, pid INTEGER NOT NULL,
	key TEXT NOT NULL, term TEXT NOT NULL, text TEXT NOT NULL,
	PRIMARY KEY(pub_id, docid, pid, key)
);
CREATE INDEX IF NOT EXISTS glossary_key ON glossary(key);

-- Every subtitle cue of every transcript ever fetched, with the millisecond it
-- starts at, so a phrase can be found in a video and opened at the right second.
CREATE TABLE IF NOT EXISTS cue(
	key TEXT NOT NULL, lang TEXT NOT NULL, seq INTEGER NOT NULL,
	start_ms INTEGER NOT NULL, end_ms INTEGER NOT NULL, text TEXT NOT NULL,
	PRIMARY KEY(key, lang, seq)
);
CREATE VIRTUAL TABLE IF NOT EXISTS cue_fts USING fts5(
	text, tokenize='unicode61 remove_diacritics 2');
-- cue_fts is kept in step by hand rather than by triggers: the rowid has to be a
-- stable handle back to a (key, lang, seq), and a contentless external-content
-- table cannot give that across the composite key.
CREATE TABLE IF NOT EXISTS cue_map(
	rowid_ INTEGER PRIMARY KEY, key TEXT NOT NULL, lang TEXT NOT NULL, seq INTEGER NOT NULL
);

-- The book names of every Bible ever indexed, so references can be read and
-- written in the library's own language instead of a table shipped per language.
CREATE TABLE IF NOT EXISTS book_name(
	lang TEXT NOT NULL, book INTEGER NOT NULL, name TEXT NOT NULL,
	PRIMARY KEY(lang, book)
);
`

// verseSchema is kept apart so the verse table can be rebuilt on its own: it is
// the one table whose layout changed after libraries existed in the wild.
const verseSchema = `
-- A library can hold more than one Bible — a study edition and a plain one, and
-- the glossary only ships with the plain one. They share BibleVerseId, so that
-- id cannot be the primary key: the second Bible indexed would silently take
-- over the first one's verses, which is exactly what happened. Each row is
-- therefore keyed by its own rowid, unique per (verse, publication), and reads
-- name the Bible they want.
CREATE TABLE IF NOT EXISTS verse(
	row_id INTEGER PRIMARY KEY,
	id INTEGER NOT NULL,               -- BibleVerseId
	book INTEGER NOT NULL, chapter INTEGER NOT NULL, verse INTEGER NOT NULL,
	text TEXT NOT NULL,
	pub_id INTEGER NOT NULL,
	UNIQUE(id, pub_id)
);
CREATE INDEX IF NOT EXISTS verse_bcv ON verse(book, chapter, verse);
CREATE INDEX IF NOT EXISTS verse_pub ON verse(pub_id, id);
CREATE VIRTUAL TABLE IF NOT EXISTS verse_fts USING fts5(
	text, content='verse', content_rowid='row_id', tokenize='unicode61 remove_diacritics 2');
CREATE TRIGGER IF NOT EXISTS verse_ai AFTER INSERT ON verse BEGIN
	INSERT INTO verse_fts(rowid, text) VALUES (new.row_id, new.text);
END;
CREATE TRIGGER IF NOT EXISTS verse_ad AFTER DELETE ON verse BEGIN
	INSERT INTO verse_fts(verse_fts, rowid, text) VALUES ('delete', old.row_id, old.text);
END;
`

const schema = verseSchema + `
CREATE TABLE IF NOT EXISTS pub(
	id INTEGER PRIMARY KEY,
	key TEXT NOT NULL UNIQUE,          -- mwb_S_202609, nwtsty_S
	symbol TEXT NOT NULL,              -- symbol for the API: mwb, w, nwtsty
	issue TEXT NOT NULL DEFAULT '',    -- 202609, 20130115
	lang TEXT NOT NULL,
	meps_symbol TEXT, undated_symbol TEXT, meps_lang INTEGER, year INTEGER, issue_tag INTEGER,
	title TEXT, short_title TEXT, pub_type TEXT, category TEXT,
	file TEXT, md5 TEXT, size INTEGER, modified TEXT, synced_at TEXT,
	first_date INTEGER, last_date INTEGER
);
CREATE TABLE IF NOT EXISTS doc(
	docid INTEGER PRIMARY KEY,         -- MepsDocumentId: the same id wol uses
	pub_id INTEGER NOT NULL,
	local_id INTEGER NOT NULL,
	class INTEGER, type INTEGER, section INTEGER, chapter INTEGER,
	title TEXT, toc_title TEXT, context_title TEXT, feature_title TEXT,
	first_page INTEGER, last_page INTEGER,
	html TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS doc_pub ON doc(pub_id);
CREATE TABLE IF NOT EXISTS par(
	id INTEGER PRIMARY KEY,
	docid INTEGER NOT NULL,
	pid INTEGER NOT NULL,
	num INTEGER, sub INTEGER,
	kind TEXT NOT NULL,
	text TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS par_doc ON par(docid, pid);
CREATE VIRTUAL TABLE IF NOT EXISTS par_fts USING fts5(
	text, content='par', content_rowid='id', tokenize='unicode61 remove_diacritics 2');
CREATE TABLE IF NOT EXISTS verse_note(      -- study notes (VerseCommentary)
	verse_id INTEGER NOT NULL, seq INTEGER NOT NULL,
	label TEXT, text TEXT NOT NULL, html TEXT NOT NULL, docid INTEGER, pub_id INTEGER NOT NULL,
	-- The paragraphs of docid this note occupies. A study Bible's note is its own
	-- document, but a study guide's "note" is a BLOCK of an index document, and
	-- the pointers to other publications are the extracts inside that block. Without
	-- the range there is no way to join the two, which is why they are kept.
	begin_pid INTEGER, end_pid INTEGER
);
CREATE INDEX IF NOT EXISTS verse_note_v ON verse_note(verse_id);
CREATE TABLE IF NOT EXISTS verse_fn(        -- footnotes of the Bible text
	verse_id INTEGER NOT NULL, fnid INTEGER NOT NULL, marker TEXT, anchor TEXT,
	text TEXT NOT NULL, pub_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS verse_fn_v ON verse_fn(verse_id);
CREATE TABLE IF NOT EXISTS verse_xref(      -- marginal references
	verse_id INTEGER NOT NULL, mid INTEGER NOT NULL, marker TEXT, anchor TEXT,
	seq INTEGER NOT NULL, first INTEGER NOT NULL, last INTEGER NOT NULL, pub_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS verse_xref_v ON verse_xref(verse_id);
CREATE TABLE IF NOT EXISTS cite(            -- BibleCitation of every document
	docid INTEGER NOT NULL, pid INTEGER, first INTEGER NOT NULL, last INTEGER NOT NULL, pub_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS cite_first ON cite(first);
CREATE INDEX IF NOT EXISTS cite_doc ON cite(docid);
CREATE TABLE IF NOT EXISTS media(
	pub_id INTEGER NOT NULL, docid INTEGER NOT NULL, mm_id INTEGER NOT NULL,
	begin_pid INTEGER, end_pid INTEGER,
	data_type INTEGER, mime TEXT, width INTEGER, height INTEGER,
	label TEXT, caption TEXT, category INTEGER, file TEXT,
	key_symbol TEXT, track INTEGER, meps_docid INTEGER, issue_tag INTEGER
);
CREATE INDEX IF NOT EXISTS media_doc ON media(docid);
CREATE TABLE IF NOT EXISTS extract(         -- text of referenced paragraphs shipped inside a pub
	pub_id INTEGER NOT NULL, docid INTEGER NOT NULL, ext_id INTEGER NOT NULL,
	begin_pid INTEGER, end_pid INTEGER, sort INTEGER,
	link TEXT, caption TEXT, title TEXT,
	ref_docid INTEGER, ref_class INTEGER, ref_begin INTEGER, ref_end INTEGER,
	ref_symbol TEXT, ref_undated TEXT, ref_issue_tag INTEGER, ref_title TEXT,
	html TEXT
);
CREATE INDEX IF NOT EXISTS extract_doc ON extract(docid);
CREATE INDEX IF NOT EXISTS extract_ref ON extract(ref_docid);
CREATE TABLE IF NOT EXISTS dated(
	pub_id INTEGER NOT NULL, docid INTEGER NOT NULL,
	first INTEGER NOT NULL, last INTEGER NOT NULL, link TEXT, caption TEXT
);
CREATE INDEX IF NOT EXISTS dated_first ON dated(first);
CREATE TABLE IF NOT EXISTS question(
	docid INTEGER NOT NULL, idx INTEGER, pid INTEGER, target_pid INTEGER, target_num INTEGER, text TEXT
);
CREATE INDEX IF NOT EXISTS question_doc ON question(docid);
CREATE TRIGGER IF NOT EXISTS par_ai AFTER INSERT ON par BEGIN
	INSERT INTO par_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER IF NOT EXISTS par_ad AFTER DELETE ON par BEGIN
	INSERT INTO par_fts(par_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;
CREATE TABLE IF NOT EXISTS video(
	key TEXT NOT NULL, lang TEXT NOT NULL, title TEXT, duration REAL, subtitles TEXT,
	json TEXT, fetched_at TEXT,
	PRIMARY KEY(key, lang)
);
`
