package store

import (
	"testing"
	"time"
)

// PlaceDocuments enriches every row it found with a second and a third query.
// The pool holds one connection, so it has to close its rows before asking
// again — this test would hang rather than fail if that ordering were lost,
// which is why it runs against a deadline.
func TestPlaceDocumentsDoesNotDeadlock(t *testing.T) {
	s := placesFixture(t)
	defer s.Close()

	done := make(chan []PlaceSource, 1)
	go func() {
		got, err := s.PlaceDocuments(20012, 20012)
		if err != nil {
			t.Error(err)
			close(done)
			return
		}
		done <- got
	}()
	select {
	case got, ok := <-done:
		if !ok {
			t.FailNow()
		}
		if len(got) != 2 {
			t.Fatalf("want the atlas map and the appendix figure, got %d: %+v", len(got), got)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("PlaceDocuments did not return: it is waiting for the connection it is holding")
	}
}

func TestPlaceDocumentsKindsAndEvidence(t *testing.T) {
	s := placesFixture(t)
	defer s.Close()

	got, err := s.PlaceDocuments(20012, 20012)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]PlaceSource{}
	for _, p := range got {
		if p.Source == "" {
			t.Errorf("a place with no evidence behind it: %+v", p)
		}
		if p.URL == "" {
			t.Errorf("a place nobody can open: %+v", p)
		}
		byKind[p.Kind] = p
	}

	atlas, ok := byKind["atlas_map"]
	if !ok {
		t.Fatalf("no atlas map; got %+v", got)
	}
	if len(atlas.Images) != 1 || atlas.Images[0].Mime != "image/jpeg" {
		t.Errorf("the atlas map should carry its picture: %+v", atlas.Images)
	}
	// An atlas map is reached through the chapter it cites, so its evidence names
	// the citation table.
	if atlas.Names == nil {
		t.Error("Names must be an empty list, never nil: it is serialised to JSON")
	}

	fig, ok := byKind["appendix_figure"]
	if !ok {
		t.Fatalf("no appendix figure; got %+v", got)
	}
	// An appendix figure only counts when it is a vector drawing: a photograph
	// in an appendix is not a map.
	if len(fig.Images) != 1 || fig.Images[0].Mime != "image/svg+xml" {
		t.Errorf("an appendix figure should keep only its svg: %+v", fig.Images)
	}
	if len(fig.Names) != 2 || fig.Names[0] != "First invented place" {
		t.Errorf("the list items of an appendix are its place names, in order: %+v", fig.Names)
	}
	// The caption comes out as text, not as the markup it is stored in.
	if fig.Images[0].Caption != "An invented caption" {
		t.Errorf("caption = %q; markup should be stripped", fig.Images[0].Caption)
	}
}

// A chapter no atlas map cites still has the library's appendix figures, which
// is the documented behaviour: the format does not tie appendices to chapters.
func TestPlaceDocumentsAppendicesAreLibraryWide(t *testing.T) {
	s := placesFixture(t)
	defer s.Close()

	got, err := s.PlaceDocuments(1001, 1001) // a chapter nothing in gl cites
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "appendix_figure" {
		t.Fatalf("want only the appendix figure, got %+v", got)
	}
}

func placesFixture(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	ex := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// The atlas, whose maps are reached through the chapters they cite.
	ex(`INSERT INTO pub(id, key, symbol, undated_symbol, lang, meps_symbol, title, year) VALUES(1,'gl_E','gl','gl','E','gl','Invented Atlas',2026)`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(301,1,1,13,'An Invented Map','')`)
	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(301,1,20001,20031,1)`)
	ex(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, data_type, mime, width, height, label, caption, category, file)
	    VALUES(1,301,1,1,2,'image/jpeg',900,600,'','A map caption',8,'gl_E_map_01.jpg')`)

	// A study Bible, whose appendices carry vector figures and lists of names.
	ex(`INSERT INTO pub(id, key, symbol, undated_symbol, lang, meps_symbol, title, year) VALUES(2,'nwtsty_E','nwtsty','nwtsty','E','nwtsty','Invented Study Bible',2026)`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(401,2,1,14,'An Invented Appendix','')`)
	ex(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, data_type, mime, label, caption, category, file)
	    VALUES(2,401,1,1,2,'image/svg+xml','','<em>An invented caption</em>',8,'nwtsty_E_fig_01.svg')`)
	// A photograph in the same appendix must not be mistaken for a map.
	ex(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, data_type, mime, label, caption, category, file)
	    VALUES(2,401,2,2,2,'image/jpeg','','A photograph',8,'nwtsty_E_photo_01.jpg')`)
	ex(`INSERT INTO par(docid, pid, kind, text) VALUES(401,1,'li','First invented place')`)
	ex(`INSERT INTO par(docid, pid, kind, text) VALUES(401,2,'li','Second invented place')`)
	ex(`INSERT INTO par(docid, pid, kind, text) VALUES(401,3,'p','A paragraph, which is not a place name')`)
	return s
}
