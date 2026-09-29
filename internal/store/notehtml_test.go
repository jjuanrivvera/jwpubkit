package store

import (
	"strings"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
)

// A study note's HTML is read to find the dictionary links in it. The fixtures
// used to leave that column empty, so nothing exercised the read — and against a
// real library the call never returned.
func TestVersesReadNoteHTML(t *testing.T) {
	s, err := Open(t.TempDir(), "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	id, _ := bible.VerseID(40, 3, 8)
	html := `<p>Una nota con una remisión (ver glosario, ` +
		`<a class="xt" href="jwpub://p/S:1001077253/"><em>arrepentimiento</em></a>). ` +
		strings.Repeat("Texto inventado para que la nota tenga cuerpo. ", 12) + `</p>`
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pub(key, symbol, issue, lang, title) VALUES('nwtsty_S','nwtsty','','S','Biblia')`, nil},
		{`INSERT INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,40,3,8,'Texto del versículo.',1)`, []any{id}},
		{`INSERT INTO verse_note(verse_id, seq, label, text, html, docid, pub_id) VALUES(?,1,'3:8','La nota.',?,1001070603,1)`, []any{id, html}},
	} {
		if _, err := s.DB.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan struct{})
	var vs []Verse
	go func() {
		defer close(done)
		vs, err = s.Verses(id, id)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Verses did not return: reading the note HTML blocks")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || len(vs[0].Notes) != 1 {
		t.Fatalf("verses = %+v", vs)
	}
	if len(vs[0].Notes[0].Defines) != 1 || vs[0].Notes[0].Defines[0].Term != "arrepentimiento" {
		t.Errorf("the note's dictionary link was not read: %+v", vs[0].Notes[0].Defines)
	}
}
