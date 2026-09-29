package cli

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func TestChapterRanges(t *testing.T) {
	rs, err := chapterRanges("Gén 37-41")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 5 {
		t.Fatalf("want 5 chapters, got %d: %+v", len(rs), rs)
	}
	for i, r := range rs {
		if r.Book != 1 || r.StartChapter != 37+i || r.EndChapter != 37+i {
			t.Fatalf("chapter %d = %+v", i, r)
		}
	}

	rs, err = chapterRanges("Jer 38-39")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].StartChapter != 38 || rs[1].StartChapter != 39 {
		t.Fatalf("Jer 38-39 = %+v", rs)
	}

	// Repeats across ranges are not duplicated.
	rs, err = chapterRanges("Jer 38-39; 39")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("dedup failed: %+v", rs)
	}
}

func TestExtractPlaceCandidates(t *testing.T) {
	texts := []string{
		"Ébed-Mélec rescató a Jeremías cerca de Jerusalén.",
		"Aunque Dios ayudó a Ébed-Mélec, nadie más actuó.",
		"El personaje visitó la ciudad de prueba otra vez.",
	}
	got := extractPlaceCandidates(texts)
	want := []string{"Jeremías", "Jerusalén", "Ébed-Mélec"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("extractPlaceCandidates = %v, want %v", got, want)
	}
}

// seedChapterFixture writes directly at the store's SQL layer (its schema is
// the stable internal contract; see internal/store/store.go) instead of
// round-tripping a synthetic JWPUB: it isolates the dedup/scoring logic
// under test from the indexer, which already has its own coverage in
// internal/store/store_test.go.
func seedChapterFixture(t *testing.T, st *store.Store) {
	t.Helper()
	db := st.DB
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	// Publications: the Bible, two citing documents (one older, one newer)
	// and Perspicacia (it) with one matching entry.
	exec(`INSERT INTO pub(id, key, symbol, issue, lang, meps_symbol, year) VALUES(1,'nwtsty_S','nwtsty','','S','nwtsty',2013)`)
	exec(`INSERT INTO pub(id, key, symbol, issue, lang, meps_symbol, year) VALUES(2,'w13_S','w','','S','w13',2013)`)
	exec(`INSERT INTO pub(id, key, symbol, issue, lang, meps_symbol, year) VALUES(3,'w24_S','w','','S','w24',2024)`)
	exec(`INSERT INTO pub(id, key, symbol, issue, lang, meps_symbol, year) VALUES(4,'it_S','it','','S','it',2018)`)

	v6, _ := bible.VerseID(24, 38, 6)
	v7, _ := bible.VerseID(24, 38, 7)
	exec(`INSERT INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,24,38,6,'Texto del versículo 6.',1)`, v6)
	exec(`INSERT INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,24,38,7,'Texto del versículo 7.',1)`, v7)

	// A study note mentioning a place candidate (matches the "it" entry
	// below) and a docid that is NOT in the library, to exercise the
	// "not in library" detection.
	exec(`INSERT INTO verse_note(verse_id, seq, label, text, html, docid, pub_id) VALUES(?,1,'Cisterna','Lo arrojaron en la cisterna de Malkiya, cerca de Jerusalén.','',9999999,1)`, v6)
	exec(`INSERT INTO verse_fn(verse_id, fnid, marker, anchor, text, pub_id) VALUES(?,1,'a','fango','O «lodo». Cerca de Jerusalén.',1)`, v6)

	// doc 2013043 (w13, older) cites only verse 6, in pid 22.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(2013043,2,1,40,'Sea valiente','')`)
	exec(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(2013043,22,12,0,'p','Ébed-Mélec fue valiente al rescatar a Jeremías.')`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(2013043,22,?,?,2)`, v6, v6)

	// doc 2024100 (w24, newer) cites both verse 6 and verse 7 in pid 5:
	// higher verse count must outrank recency alone, and among ties
	// recency must win.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(2024100,3,1,40,'Hagamos amistades fuertes','')`)
	exec(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(2024100,5,3,0,'p','Jeremías y Ébed-Mélec: un ejemplo doble.')`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(2024100,5,?,?,3)`, v6, v6)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(2024100,5,?,?,3)`, v7, v7)
	exec(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, end_pid, data_type, mime, width, height, label, caption, category, file)
		VALUES(3,2024100,1,5,5,0,'image/jpeg',1200,675,'','Ébed-Mélec habla con el rey.',8,'2024100_univ_cnt_1.jpg')`)

	// A citation without an indexed paragraph: must surface as "sin
	// extracto" instead of failing.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(2020200,3,2,40,'Otro artículo','')`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(2020200,99,?,?,3)`, v7, v7)

	// Perspicacia entry matching the note's "Jerusalén" candidate.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(1200000500,4,1,2,'Jerusalén','')`)
}

func TestBuildChapterDossier(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedChapterFixture(t, st)

	a := &app{}
	r, err := bible.Parse("Jer 38")
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.buildChapterDossier(st, r[0], true)
	if err != nil {
		t.Fatal(err)
	}

	if d.VerseCount != 2 {
		t.Fatalf("VerseCount = %d, want 2", d.VerseCount)
	}
	if d.CiteCount != 3 {
		t.Fatalf("CiteCount = %d, want 3: %+v", d.CiteCount, d.Citations)
	}

	// Scoring: doc 2024100 cites 2 distinct verses, so it must rank first
	// even though doc 2013043 (w13) is a valid citation too.
	if d.Citations[0].DocID != 2024100 || len(d.Citations[0].Verses) != 2 {
		t.Fatalf("top citation = %+v", d.Citations[0])
	}
	if d.Citations[0].Extract == "" || d.Citations[0].NoExtract {
		t.Fatalf("expected extract text for the top citation: %+v", d.Citations[0])
	}

	// The citation with no indexed paragraph must be flagged, not fail.
	var noExtract *citationOut
	for i := range d.Citations {
		if d.Citations[i].DocID == 2020200 {
			noExtract = &d.Citations[i]
		}
	}
	if noExtract == nil || !noExtract.NoExtract {
		t.Fatalf("citation without a paragraph must be marked sin_extracto: %+v", d.Citations)
	}

	// Images come only from cited documents (2024100), never downloaded.
	if d.ImageCount != 1 || d.Images[0].DocID != 2024100 || d.Images[0].Width != 1200 {
		t.Fatalf("Images = %+v", d.Images)
	}

	// Places: "Jerusalén" appears in both the note and the footnote (dedup)
	// and matches the it entry; the stopword-filtered/position-0 words must
	// not appear.
	foundJerusalen := false
	for _, p := range d.Places {
		if p.Name == "Jerusalén" {
			foundJerusalen = true
			if !p.InIt || p.ItDocID != 1200000500 {
				t.Fatalf("Jerusalén should cross-reference it docid 1200000500: %+v", p)
			}
		}
	}
	if !foundJerusalen {
		t.Fatalf("Jerusalén missing from places: %+v", d.Places)
	}

	// "Not in library": the study note's docid 9999999 is not synced.
	foundMissingDoc := false
	for _, m := range d.NotInLib {
		if strings.Contains(m, "9999999") {
			foundMissingDoc = true
		}
	}
	if !foundMissingDoc {
		t.Fatalf("expected a not-in-library notice for docid 9999999: %v", d.NotInLib)
	}
}

func TestBuildChapterDossierMissingBible(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &app{}
	r, err := bible.Parse("Jer 38")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.buildChapterDossier(st, r[0], false); err == nil {
		t.Fatal("expected an error when the chapter has no verses in the library")
	}
}

func TestBuildChapterDossierNoItSynced(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedChapterFixture(t, st)
	a := &app{}
	r, _ := bible.Parse("Jer 38")
	d, err := a.buildChapterDossier(st, r[0], false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Places) != 0 {
		t.Fatalf("places must stay empty when it is not synced: %+v", d.Places)
	}
	found := false
	for _, m := range d.NotInLib {
		if strings.Contains(m, "(it) is not synced") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an explicit notice about it not being synced: %v", d.NotInLib)
	}
}

func TestRenderChapterDossierNeverDropsVersesOrCitations(t *testing.T) {
	d := &chapterDossier{
		Ref:        "Jeremías 38",
		VerseCount: 2,
		Verses: []store.Verse{
			{Book: 24, Chapter: 38, Verse: 6, Text: "Texto 6."},
			{Book: 24, Chapter: 38, Verse: 7, Text: "Texto 7."},
		},
	}
	for i := 0; i < 5; i++ {
		d.Citations = append(d.Citations, citationOut{
			DocID: 2000000 + i, Pub: "w24", Title: "Doc", Verses: []int{6},
			Extract: strings.Repeat("palabra clave del párrafo citado ", 200), // long enough to force trimming
		})
	}
	d.CiteCount = len(d.Citations)

	md := renderChapterDossier(d)

	if d.TokenEst <= 0 {
		t.Fatalf("TokenEst not set")
	}
	if !d.ExtractsCut {
		t.Fatalf("expected extracts to be cut for a chapter this large")
	}
	for i := range d.Citations {
		docid := 2000000 + i
		if !strings.Contains(md, "docid "+strconv.Itoa(docid)) {
			t.Fatalf("citation docid %d dropped from the rendered dossier", docid)
		}
	}
	for _, v := range d.Verses {
		if !strings.Contains(md, v.Text) {
			t.Fatalf("verse %d:%d dropped from the rendered dossier", v.Chapter, v.Verse)
		}
	}
}

func TestChapterDossierJSONShape(t *testing.T) {
	d := &chapterDossier{
		Ref: "Jeremías 38", VerseCount: 1, CiteCount: 1, ImageCount: 0, PlaceCount: 0,
		Citations: []citationOut{{DocID: 1, Pub: "w24", Title: "T", Verses: []int{6}, Extract: "x"}},
		NotInLib:  []string{"algo no está"},
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"reference", "verse_count", "citation_count", "citations", "not_in_library"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing JSON key %q in %s", key, b)
		}
	}
	cites, ok := m["citations"].([]any)
	if !ok || len(cites) != 1 {
		t.Fatalf("citations shape: %s", b)
	}
	first := cites[0].(map[string]any)
	for _, key := range []string{"docid", "publication", "title", "verses", "extract"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("missing citation JSON key %q in %s", key, b)
		}
	}
}
