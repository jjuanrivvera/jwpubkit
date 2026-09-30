package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// queryFixture is a two-publication library: a magazine with two articles and
// an "insight"-style reference work, plus the rows the read queries need. It is
// all invented; the shape is what matters, never the words.
func queryFixture(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	ex := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ex(`INSERT INTO pub(id, key, symbol, undated_symbol, issue, lang, meps_symbol, title, short_title, pub_type, file, md5, size, synced_at, year)
	    VALUES(1,'w_E_202601','w','w','202601','E','w26.01','Invented Magazine 2026','Invented Mag','Magazine','/tmp/w.jwpub','abc',1024,'2026-01-01T00:00:00Z',2026)`)
	ex(`INSERT INTO pub(id, key, symbol, undated_symbol, issue, lang, meps_symbol, title, file, year)
	    VALUES(2,'it_E','it-1','it','','E','it-1','Invented Reference Work','/tmp/it.jwpub',2026)`)

	ex(`INSERT INTO doc(docid, pub_id, local_id, class, chapter, title, context_title, html) VALUES(101,1,1,40,3,'First Invented Article','Study Article 1','<p>a</p>')`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(102,1,2,40,'Second Invented Article','<p>b</p>')`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(103,1,3,13,'Not A Study Article','')`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(201,2,1,13,'  Invented Term  ','')`)

	ex(`INSERT INTO par(docid, pid, kind, text) VALUES(101,5,'p','The first half of a paragraph.')`)
	ex(`INSERT INTO par(docid, pid, kind, text) VALUES(101,5,'p','And the second half.')`)
	ex(`INSERT INTO par(docid, pid, kind, text) VALUES(101,6,'p','Another paragraph entirely.')`)

	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,5,20012,20012,1)`)
	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,6,20013,20014,1)`)
	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,0,99999999,99999999,1)`)

	ex(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, end_pid, data_type, mime, width, height, label, caption, category, file)
	    VALUES(1,101,2,6,6,2,'image/jpeg',1200,675,'Second picture','A caption',8,'1102026001_E_art_02.jpg')`)
	ex(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, end_pid, data_type, mime, width, height, label, caption, category, file)
	    VALUES(1,101,1,5,5,2,'image/jpeg',600,400,'First picture','',8,'1102026001_E_art_01.jpg')`)

	ex(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, link, caption, title, ref_docid, ref_class, ref_begin, ref_end, ref_symbol, ref_undated, ref_issue_tag, ref_title, html, sort)
	    VALUES(1,101,1,7,'p/E:201/1-1','box on page 4','First Invented Article',201,13,1,1,'it-1','it',0,'Invented Reference Work','<p>short</p>',2)`)
	ex(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, link, caption, title, ref_docid, ref_class, ref_begin, ref_end, ref_symbol, ref_undated, ref_issue_tag, ref_title, html, sort)
	    VALUES(1,101,2,3,'p/E:201/2-2','box on page 2','First Invented Article',201,13,2,2,'it-1','it',0,'Invented Reference Work','<p>a much longer quoted passage</p>',1)`)

	ex(`INSERT INTO dated(pub_id, docid, first, last, link) VALUES(1,101,20260105,20260111,'a')`)
	ex(`INSERT INTO dated(pub_id, docid, first, last, link) VALUES(1,102,20260112,20260118,'b')`)
	return s
}

func TestPubByKeyAndHasSymbol(t *testing.T) {
	s := queryFixture(t)

	p, err := s.PubByKey("w_E_202601")
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.Symbol != "w" || p.MepsSymbol != "w26.01" || p.Size != 1024 {
		t.Fatalf("PubByKey = %+v", p)
	}

	// A key that is not in the library is an absence, not an error: the caller
	// decides whether to sync it.
	missing, err := s.PubByKey("w_E_209912")
	if err != nil || missing != nil {
		t.Errorf("PubByKey of an unsynced publication = %+v, %v; want nil, nil", missing, err)
	}

	if !s.HasSymbol("w") || !s.HasSymbol("it-1") {
		t.Error("HasSymbol should find what is synced")
	}
	if s.HasSymbol("mwb") {
		t.Error("HasSymbol should not invent a publication")
	}
}

// The chapter number is the language-neutral handle on a document, so absence
// has to be distinguishable from zero.
func TestChapterNumber(t *testing.T) {
	s := queryFixture(t)
	if n, ok := s.ChapterNumber(101); !ok || n != 3 {
		t.Errorf("ChapterNumber(101) = %d, %v; want 3, true", n, ok)
	}
	// No number to give reads the same whether the document carries no chapter
	// or is not in the library at all.
	if n, ok := s.ChapterNumber(102); ok || n != 0 {
		t.Errorf("a document with no chapter should report 0, false: got %d, %v", n, ok)
	}
	if _, ok := s.ChapterNumber(9999); ok {
		t.Error("a document that is not in the library should report false")
	}
}

func TestSummaries(t *testing.T) {
	s := queryFixture(t)
	got, err := s.Summaries([]int{101, 201, 9999})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want the two that exist, got %d: %+v", len(got), got)
	}
	if got[101].Title != "First Invented Article" || got[101].Pub != "w26.01" || got[101].Context != "Study Article 1" {
		t.Errorf("summary 101 = %+v", got[101])
	}
	if out, err := s.Summaries(nil); err != nil || len(out) != 0 {
		t.Errorf("Summaries(nil) = %+v, %v", out, err)
	}
}

// One pid can be stored as several paragraph rows; the reader must get one
// paragraph back, in order, not the first fragment.
func TestParagraphText(t *testing.T) {
	s := queryFixture(t)
	got, err := s.ParagraphText(101, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got != "The first half of a paragraph. And the second half." {
		t.Errorf("ParagraphText = %q", got)
	}
	if got, err := s.ParagraphText(101, 99); err != nil || got != "" {
		t.Errorf("a pid with no text = %q, %v", got, err)
	}
}

// The title is matched with the whitespace and case a publication happens to
// use, because what is being matched is a term the reader typed.
func TestDocByTitle(t *testing.T) {
	s := queryFixture(t)
	for _, sym := range []string{"it", "it-1"} {
		d, err := s.DocByTitle(sym, "invented TERM")
		if err != nil {
			t.Fatal(err)
		}
		if d == nil || d.DocID != 201 {
			t.Errorf("DocByTitle(%q) = %+v", sym, d)
		}
	}
	d, err := s.DocByTitle("it", "a term nobody wrote")
	if err != nil || d != nil {
		t.Errorf("an unknown title = %+v, %v; want nil, nil", d, err)
	}
}

// Images come back in reading order, which is the paragraph they sit in — not
// the order the publication's tables happen to store them in.
func TestMediaOf(t *testing.T) {
	s := queryFixture(t)
	got, err := s.MediaOf(101)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 pictures, got %d", len(got))
	}
	if got[0].Label != "First picture" {
		t.Errorf("out of reading order: %+v", []string{got[0].Label, got[1].Label})
	}
	if got[0].Width != 600 || got[0].Mime != "image/jpeg" || got[0].PubFile != "/tmp/w.jwpub" {
		t.Errorf("first picture = %+v", got[0])
	}
	if out, err := s.MediaOf(9999); err != nil || len(out) != 0 {
		t.Errorf("MediaOf of a document with no media = %+v, %v", out, err)
	}
}

func TestExtracts(t *testing.T) {
	s := queryFixture(t)

	// Inside a document, extracts read in the publication's own order.
	of, err := s.ExtractsOf(101)
	if err != nil {
		t.Fatal(err)
	}
	if len(of) != 2 || of[0].BeginPID != 3 {
		t.Fatalf("ExtractsOf out of order: %+v", of)
	}
	if of[0].RefDocID != 201 || of[0].RefClass != 13 || of[0].RefTitle == "" {
		t.Errorf("extract lost its reference: %+v", of[0])
	}

	// Looking the other way — who quotes this document — the longest quotation
	// is the most useful one, so it comes first.
	quoting, err := s.ExtractsFor(201)
	if err != nil {
		t.Fatal(err)
	}
	if len(quoting) != 2 {
		t.Fatalf("want both quotations, got %d", len(quoting))
	}
	if len(quoting[0].HTML) < len(quoting[1].HTML) {
		t.Errorf("the longest quotation should come first: %q then %q", quoting[0].HTML, quoting[1].HTML)
	}
}

// M³ maps a week to a study article by position, so only class-40 documents
// count and the order must be the publication's.
func TestStudyArticlesAndDatedRank(t *testing.T) {
	s := queryFixture(t)
	arts, err := s.StudyArticles("w_E_202601")
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 || arts[0] != 101 || arts[1] != 102 {
		t.Errorf("StudyArticles = %v; want the two class-40 documents in order", arts)
	}

	for first, want := range map[int]int{20260105: 0, 20260112: 1} {
		got, err := s.DatedRank("w_E_202601", first)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("DatedRank(%d) = %d, want %d", first, got, want)
		}
	}
}

func TestVideoCache(t *testing.T) {
	s := queryFixture(t)

	if v, err := s.Video("pub-inv-1_1_VIDEO", "E"); err != nil || v != nil {
		t.Fatalf("an empty cache should report nothing: %+v, %v", v, err)
	}
	if err := s.PutVideo("pub-inv-1_1_VIDEO", "E", "Invented Video", 91.5, "https://example.invalid/a.vtt", map[string]any{"guid": "x"}); err != nil {
		t.Fatal(err)
	}
	v, err := s.Video("pub-inv-1_1_VIDEO", "E")
	if err != nil || v == nil {
		t.Fatalf("Video = %+v, %v", v, err)
	}
	if v.Title != "Invented Video" || v.Duration != 91.5 || !strings.Contains(v.JSON, `"guid"`) {
		t.Errorf("cached video = %+v", v)
	}
	// The cache is keyed by language too: the same video has a title per language.
	if other, err := s.Video("pub-inv-1_1_VIDEO", "S"); err != nil || other != nil {
		t.Errorf("another language should miss the cache: %+v, %v", other, err)
	}
	// Re-caching replaces rather than duplicating.
	if err := s.PutVideo("pub-inv-1_1_VIDEO", "E", "Renamed", 91.5, "", nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Video("pub-inv-1_1_VIDEO", "E"); v == nil || v.Title != "Renamed" {
		t.Errorf("PutVideo should replace: %+v", v)
	}
}

// The citation table groups by paragraph, and a reference the Bible numbering
// does not recognise is dropped rather than turned into a bogus range.
func TestScripturesOf(t *testing.T) {
	s := queryFixture(t)
	got, err := s.ScripturesOf(101)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[5]) != 1 || len(got[6]) != 1 {
		t.Fatalf("ScripturesOf = %+v", got)
	}
	if _, ok := got[0]; ok {
		t.Errorf("an unparseable verse id should be dropped, not grouped: %+v", got[0])
	}
	if out, err := s.ScripturesOf(9999); err != nil || len(out) != 0 {
		t.Errorf("ScripturesOf of a document with no citations = %+v, %v", out, err)
	}
}

// The three files are one unit; a caller that removes a library must be handed
// all of them, or the leftovers get applied to the next database created.
func TestFilesAndSize(t *testing.T) {
	s := queryFixture(t)
	files := s.Files()
	if len(files) != 3 {
		t.Fatalf("Files = %v", files)
	}
	if filepath.Base(files[0]) != filepath.Base(s.Path) {
		t.Errorf("the first file should be the database itself: %v", files)
	}
	if !strings.HasSuffix(files[1], "-wal") || !strings.HasSuffix(files[2], "-shm") {
		t.Errorf("the -wal and -shm must be in the set: %v", files)
	}
	if s.Size() <= 0 {
		t.Error("a library with publications in it should measure more than nothing")
	}
}

// Without a commentary index CommentedOn can answer nothing, and saying so is
// the difference between "no commentary exists" and "we cannot look".
func TestHasCommentaryIndex(t *testing.T) {
	s := queryFixture(t)
	if s.HasCommentaryIndex() {
		t.Error("a library with no verse_note rows cannot index commentary")
	}
	if _, err := s.DB.Exec(`INSERT INTO verse_note(verse_id, seq, text, html, docid, pub_id, begin_pid, end_pid) VALUES(20012,1,'x','<p>x</p>',101,1,5,5)`); err != nil {
		t.Fatal(err)
	}
	if !s.HasCommentaryIndex() {
		t.Error("a verse_note with a paragraph range is exactly what makes it possible")
	}
}

func TestGlossaryTermsAndSize(t *testing.T) {
	s := queryFixture(t)
	if n := s.GlossarySize(); n != 0 {
		t.Errorf("an empty glossary should measure 0, got %d", n)
	}
	for _, term := range []string{"Abaddon", "Abomination", "Zeal"} {
		if _, err := s.DB.Exec(`INSERT INTO glossary(pub_id, docid, pid, key, term, text) VALUES(2,201,4,?,?,?)`,
			FoldTerm(term), term, "An invented definition of "+term+"."); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.GlossarySize(); n != 3 {
		t.Errorf("GlossarySize = %d, want 3", n)
	}

	all, err := s.Terms("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Term != "Abaddon" {
		t.Errorf("Terms should list every term in order: %+v", all)
	}
	if all[0].URL == "" {
		t.Error("a term should come with the address that shows it")
	}

	// A prefix narrows the list, folded the same way the keys were.
	ab, err := s.Terms("ab", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ab) != 2 {
		t.Errorf("prefix ab = %+v", ab)
	}
	if one, err := s.Terms("", 1); err != nil || len(one) != 1 {
		t.Errorf("the limit should be honoured: %+v, %v", one, err)
	}
}
