// Package store is the local library: cached JWPUB files plus a SQLite
// database with the decrypted text, paragraph FTS5 index, Bible verses, study
// notes, citations, media and extracts of every synced publication.
package store

import (
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
	DB *sql.DB
	// Dir is the library directory, shared by every language.
	Dir string
	// Lang is the language this handle holds; Path is the file it holds it in.
	Lang string
	Path string
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
func Open(dir, lang string) (*Store, error) {
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
	s := &Store{DB: db, Dir: dir, Lang: lang, Path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	s.teachBookNames()
	return s, nil
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
-- The book names of every Bible ever indexed, so references can be read and
-- written in the library's own language instead of a table shipped per language.
CREATE TABLE IF NOT EXISTS book_name(
	lang TEXT NOT NULL, book INTEGER NOT NULL, name TEXT NOT NULL,
	PRIMARY KEY(lang, book)
);
`

const schema = `
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
CREATE TABLE IF NOT EXISTS verse(
	id INTEGER PRIMARY KEY,            -- BibleVerseId
	book INTEGER NOT NULL, chapter INTEGER NOT NULL, verse INTEGER NOT NULL,
	text TEXT NOT NULL,
	pub_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS verse_bcv ON verse(book, chapter, verse);
CREATE VIRTUAL TABLE IF NOT EXISTS verse_fts USING fts5(
	text, content='verse', content_rowid='id', tokenize='unicode61 remove_diacritics 2');
CREATE TABLE IF NOT EXISTS verse_note(      -- study notes (VerseCommentary)
	verse_id INTEGER NOT NULL, seq INTEGER NOT NULL,
	label TEXT, text TEXT NOT NULL, html TEXT NOT NULL, docid INTEGER, pub_id INTEGER NOT NULL
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
CREATE TRIGGER IF NOT EXISTS verse_ai AFTER INSERT ON verse BEGIN
	INSERT INTO verse_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER IF NOT EXISTS verse_ad AFTER DELETE ON verse BEGIN
	INSERT INTO verse_fts(verse_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;
CREATE TABLE IF NOT EXISTS video(
	key TEXT NOT NULL, lang TEXT NOT NULL, title TEXT, duration REAL, subtitles TEXT,
	json TEXT, fetched_at TEXT,
	PRIMARY KEY(key, lang)
);
`
