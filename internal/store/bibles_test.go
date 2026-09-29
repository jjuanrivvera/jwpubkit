package store

import (
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"

	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// Two Bibles share every verse id, and the glossary only ships with the plain
// edition — so holding both is the normal case, not an exotic one. Indexing the
// second used to take the first one's verses over: measured on a live library,
// where a study edition became a plain one without a word.
func TestASecondBibleDoesNotStealTheFirstsVerses(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, _ := bible.VerseID(24, 38, 6)
	study := testutil.Pub{
		Symbol: "nwtsty", Undated: "nwtsty", Year: 2026, Title: "Study edition",
		Verses: map[int]string{id: `<span id="v24-38-6" class="v">Study edition wording.</span>`},
	}
	plain := testutil.Pub{
		Symbol: "nwt", Undated: "nwt", Year: 2026, Title: "Plain edition",
		Verses: map[int]string{id: `<span id="v24-38-6" class="v">Plain edition wording.</span>`},
	}
	for _, p := range []testutil.Pub{study, plain} {
		if _, err := s.IndexLocal(testutil.Build(t, t.TempDir(), p), p.Symbol, "", "S"); err != nil {
			t.Fatal(err)
		}
	}

	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM verse WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("the library holds %d copies of the verse, want one per Bible", n)
	}

	// Both Bibles are intact, each under its own publication.
	rows, err := s.DB.Query(`SELECT p.symbol, v.text FROM verse v JOIN pub p ON p.id = v.pub_id WHERE v.id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var sym, text string
		if err := rows.Scan(&sym, &text); err != nil {
			t.Fatal(err)
		}
		got[sym] = text
	}
	if len(got) != 2 || got["nwtsty"] == got["nwt"] {
		t.Errorf("each Bible should keep its own wording: %+v", got)
	}
}

// With several Bibles present, reads come from the one that can answer most:
// the one with study notes.
func TestReadsPreferTheRichestBible(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, _ := bible.VerseID(24, 38, 6)
	plain := testutil.Pub{
		Symbol: "nwt", Undated: "nwt", Year: 2026, Title: "Plain edition",
		Verses: map[int]string{id: `<span id="v24-38-6" class="v">Plain edition wording.</span>`},
	}
	if _, err := s.IndexLocal(testutil.Build(t, t.TempDir(), plain), "nwt", "", "S"); err != nil {
		t.Fatal(err)
	}
	// One Bible: that is the one.
	if !s.HasBible() {
		t.Fatal("a library with a Bible should say so")
	}
	vs, err := s.Verses(id, id)
	if err != nil || len(vs) != 1 {
		t.Fatalf("Verses = %+v %v", vs, err)
	}
	if s.BibleTitle() != "Plain edition" {
		t.Errorf("BibleTitle = %q", s.BibleTitle())
	}

	// Add study notes to a second Bible and reads should move to it.
	var pubID int64
	if err := s.DB.QueryRow(`SELECT id FROM pub WHERE symbol='nwt'`).Scan(&pubID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO pub(key, symbol, issue, lang, title) VALUES('nwtsty_S','nwtsty','','S','Study edition')`); err != nil {
		t.Fatal(err)
	}
	var studyID int64
	if err := s.DB.QueryRow(`SELECT id FROM pub WHERE symbol='nwtsty'`).Scan(&studyID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,24,38,6,'Study edition wording.',?)`, id, studyID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO verse_note(verse_id, seq, label, text, html, docid, pub_id) VALUES(?,1,'l','note','',0,?)`, id, studyID); err != nil {
		t.Fatal(err)
	}
	s.bibleID = 0 // the choice is cached; this is a new library as far as reads go

	vs, err = s.Verses(id, id)
	if err != nil || len(vs) != 1 {
		t.Fatalf("Verses = %+v %v", vs, err)
	}
	if vs[0].Text != "Study edition wording." {
		t.Errorf("reads should come from the Bible with the notes, got %q", vs[0].Text)
	}
	if s.BibleTitle() != "Study edition" {
		t.Errorf("BibleTitle = %q", s.BibleTitle())
	}
}
