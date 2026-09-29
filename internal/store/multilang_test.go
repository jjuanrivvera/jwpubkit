package store

import (
	"path/filepath"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// A MEPS document id is the same number in every language, so two editions of
// one publication collide on it. They did: syncing a publication in a second
// language silently left the first with zero documents. Each language gets its
// own database file, which makes the collision impossible rather than forbidden.
func TestTwoLanguagesDoNotEvictEachOther(t *testing.T) {
	dir := t.TempDir()
	pub := testutil.Pub{
		Symbol: "th", Undated: "th", Year: 2026, Title: "Test publication",
		Docs: []testutil.Doc{{ID: 0, MepsID: 1102018437, Class: 40, Title: "A document",
			HTML: `<p id="p1" data-pid="1">Invented text.</p>`}},
	}
	file := testutil.Build(t, t.TempDir(), pub)

	for _, lang := range []string{"E", "S"} {
		s, err := Open(dir, lang)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.IndexLocal(file, "th", "", lang); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}

	for _, lang := range []string{"E", "S"} {
		s, err := Open(dir, lang)
		if err != nil {
			t.Fatal(err)
		}
		pubs, err := s.Pubs()
		if err != nil {
			t.Fatal(err)
		}
		if len(pubs) != 1 {
			t.Fatalf("%s: got %d publications, want exactly its own", lang, len(pubs))
		}
		if pubs[0].Docs != 1 {
			t.Errorf("%s: the publication has %d documents, want 1 — the other language evicted it", lang, pubs[0].Docs)
		}
		if _, err := s.Doc(1102018437); err != nil {
			t.Errorf("%s: its own document is gone: %v", lang, err)
		}
		s.Close()
	}
}

// A library written before languages were separated keeps working: the file it
// already holds stays the file for the language inside it, and another language
// goes somewhere else instead of landing on top of it.
func TestLegacyDatabaseKeepsItsLanguage(t *testing.T) {
	dir := t.TempDir()
	pub := testutil.Pub{
		Symbol: "th", Undated: "th", Year: 2026, Title: "Publicación de prueba",
		Docs: []testutil.Doc{{ID: 0, MepsID: 1102018437, Class: 40, Title: "Un documento",
			HTML: `<p id="p1" data-pid="1">Texto inventado.</p>`}},
	}
	file := testutil.Build(t, t.TempDir(), pub)

	// Opening with no language gives the pre-language file, which is what an
	// installation from before this change has on disk: jwlib.db, holding Spanish.
	legacy, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, legacyDB); legacy.Path != want {
		t.Fatalf("expected the pre-language file %s, got %s", want, legacy.Path)
	}
	if _, err := legacy.IndexLocal(file, "th", "", "S"); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	// That library, opened in the language it holds, must be the same library.
	s, err := Open(dir, "S")
	if err != nil {
		t.Fatal(err)
	}
	if s.Path != filepath.Join(dir, legacyDB) {
		t.Errorf("the language already in the library should keep its file, got %s", s.Path)
	}
	if pubs, _ := s.Pubs(); len(pubs) != 1 {
		t.Errorf("an existing library came back empty: %+v", pubs)
	}
	s.Close()

	// Another language must not land in that same file.
	other, err := Open(dir, "E")
	if err != nil {
		t.Fatal(err)
	}
	if other.Path == filepath.Join(dir, legacyDB) {
		t.Error("a second language reused the first one's file")
	}
	if pubs, _ := other.Pubs(); len(pubs) != 0 {
		t.Errorf("a fresh language should start empty, got %+v", pubs)
	}
	other.Close()
}

// A library that never existed starts in its own per-language file rather than
// claiming the pre-language name, so nothing later has to guess what it holds.
func TestNewLibraryUsesItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "E")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if want := filepath.Join(dir, "jwlib.E.db"); s.Path != want {
		t.Errorf("Path = %s, want %s", s.Path, want)
	}
}
