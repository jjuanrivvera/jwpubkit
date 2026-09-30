package store

import "testing"

// Every edge the graph reports has to name where it came from: the point of
// walking links the publications wrote is that the answer can be checked.
func TestVerseGraphEdgesCarryTheirSource(t *testing.T) {
	s := graphFixture(t)
	defer s.Close()

	edges, err := s.VerseGraph(20012, 20012, GraphLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatal("no edges")
	}
	byRelation := map[string][]Edge{}
	for _, e := range edges {
		if e.Source == "" {
			t.Errorf("edge without a source: %+v", e)
		}
		byRelation[e.Relation] = append(byRelation[e.Relation], e)
	}
	for _, want := range []string{"cited-by", "points-to", "cited-alongside", "video-in-citing-document"} {
		if len(byRelation[want]) == 0 {
			t.Errorf("no %s edge; got %v", want, keysOf(byRelation))
		}
	}
	// A citing document is citable and openable.
	c := byRelation["cited-by"][0]
	if c.Cite == "" || c.URL == "" || c.DocID == 0 {
		t.Errorf("cited-by edge is not usable: %+v", c)
	}
	// Co-citation counts how often two passages are taught together, and never
	// reports the passage itself.
	for _, e := range byRelation["cited-alongside"] {
		if e.Weight <= 0 {
			t.Errorf("co-citation without a weight: %+v", e)
		}
		if e.Reference == "Gé 1:1" {
			t.Error("a passage was reported as cited alongside itself")
		}
	}
}

// A much-quoted verse must be capped, not truncated by accident.
func TestVerseGraphRespectsItsCaps(t *testing.T) {
	s := graphFixture(t)
	defer s.Close()
	edges, err := s.VerseGraph(20012, 20012, GraphLimits{PerRelation: 1, CoCitation: 1})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range edges {
		counts[e.Relation]++
	}
	for rel, n := range counts {
		if n > 1 {
			t.Errorf("%s returned %d edges with a cap of 1", rel, n)
		}
	}
}

// A transcript hit is worth more when it says which article uses the video.
func TestVideoDocuments(t *testing.T) {
	s := graphFixture(t)
	defer s.Close()
	docs, err := s.VideoDocuments("pub-test_1_VIDEO", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].DocID != 101 {
		t.Fatalf("VideoDocuments = %+v", docs)
	}
	if docs[0].Relation != "used-in" || docs[0].Cite == "" {
		t.Errorf("edge is not usable: %+v", docs[0])
	}
}

func graphFixture(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), "S")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO pub(id, key, symbol, issue, lang, meps_symbol, title, year) VALUES(1,'w_S_202607','w','202607','S','w26.07','Revista',2026)`)
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(101,1,1,40,'Un artículo','')`)
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(102,1,2,40,'Otro artículo','')`)
	// Two documents quote the verse; one of them also quotes two others in the
	// same paragraph, which is what co-citation is.
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,5,20012,20012,1)`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(102,7,20012,20012,1)`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,5,20013,20013,1)`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(102,7,20013,20013,1)`)
	exec(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,5,20014,20014,1)`)
	// A margin reference out of the verse.
	exec(`INSERT INTO verse_xref(verse_id, mid, marker, anchor, seq, first, last, pub_id) VALUES(20012,1,'a','una palabra',1,20020,20020,1)`)
	// A video embedded in a document that quotes it.
	exec(`INSERT INTO doc_video(docid, pub_id, key) VALUES(101,1,'pub-test_1_VIDEO')`)
	exec(`INSERT INTO video(key, lang, title, duration, subtitles, json, fetched_at) VALUES('pub-test_1_VIDEO','S','Un video',60,'','','')`)
	return s
}

func keysOf(m map[string][]Edge) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// DocGraph is the document-shaped view of the same links: what quotes it, what
// it points at, what video it embeds and what verses it teaches. Each of the
// four comes from a different table, so each edge has to name which one.
func TestDocGraphCoversItsFourRelations(t *testing.T) {
	s := docGraphFixture(t)
	defer s.Close()

	edges, err := s.DocGraph(101, GraphLimits{})
	if err != nil {
		t.Fatal(err)
	}
	byRelation := map[string][]Edge{}
	for _, e := range edges {
		if e.Source == "" {
			t.Errorf("edge without a source: %+v", e)
		}
		byRelation[e.Relation] = append(byRelation[e.Relation], e)
	}
	for _, want := range []string{"extracted-by", "refers-to", "embeds-video", "quotes"} {
		if len(byRelation[want]) == 0 {
			t.Errorf("no %s edge; got %v", want, keysOf(byRelation))
		}
	}
	if e := byRelation["extracted-by"][0]; e.DocID != 102 || e.Cite == "" || e.URL == "" {
		t.Errorf("extracted-by is not usable: %+v", e)
	}
	if e := byRelation["refers-to"][0]; e.DocID != 201 {
		t.Errorf("refers-to should point at what the extract references: %+v", e)
	}
	// The weight of an embedded video is how many transcript lines are indexed
	// for it: that is what tells the reader whether it can be searched.
	if e := byRelation["embeds-video"][0]; e.VideoKey == "" || e.Weight != 2 || e.URL == "" {
		t.Errorf("embeds-video = %+v; want the key, 2 cues and an address", e)
	}
	// A citation the Bible numbering cannot place is dropped, not reported with
	// an empty reference.
	for _, e := range byRelation["quotes"] {
		if e.Reference == "" {
			t.Errorf("a quotes edge with no reference should not be reported: %+v", e)
		}
	}
}

func TestDocGraphRespectsItsCaps(t *testing.T) {
	s := docGraphFixture(t)
	defer s.Close()
	edges, err := s.DocGraph(101, GraphLimits{PerRelation: 1})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range edges {
		counts[e.Relation]++
	}
	for rel, n := range counts {
		if n > 1 {
			t.Errorf("%s ignored the cap: %d edges", rel, n)
		}
	}
}

// A document nothing links to is an empty answer, not an error: "nothing is
// recorded" is a finding the caller has to be able to print.
func TestDocGraphOfAnUnlinkedDocument(t *testing.T) {
	s := docGraphFixture(t)
	defer s.Close()
	edges, err := s.DocGraph(999, GraphLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 0 {
		t.Errorf("want no edges, got %+v", edges)
	}
}

func docGraphFixture(t *testing.T) *Store {
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
	ex(`INSERT INTO pub(id, key, symbol, undated_symbol, issue, lang, meps_symbol, title, year)
	    VALUES(1,'w_E_202601','w','w','202601','E','w26.01','Invented Magazine',2026)`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(101,1,1,40,'The Document Under Study','')`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(102,1,2,40,'A Document That Quotes It','')`)
	ex(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(103,1,3,40,'Another That Quotes It','')`)
	// Two documents quote 101; 101 itself points at 201 through an extract.
	ex(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, caption, title, ref_docid, ref_class, ref_symbol, html, sort)
	    VALUES(1,102,1,4,'box on page 7','A Document That Quotes It',101,40,'w26.01','<p>x</p>',1)`)
	ex(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, caption, title, ref_docid, ref_class, ref_symbol, html, sort)
	    VALUES(1,103,1,9,'box on page 9','Another That Quotes It',101,40,'w26.01','<p>y</p>',1)`)
	ex(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, caption, title, ref_docid, ref_class, ref_symbol, html, sort)
	    VALUES(1,101,1,3,'box on page 3','The Document Under Study',201,13,'it-1','<p>z</p>',1)`)
	ex(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, caption, title, ref_docid, ref_class, ref_symbol, html, sort)
	    VALUES(1,101,2,5,'box on page 5','The Document Under Study',202,13,'it-1','<p>w</p>',2)`)
	// Two videos, one of them with an indexed transcript.
	ex(`INSERT INTO doc_video(docid, pub_id, key) VALUES(101,1,'pub-inv-1_1_VIDEO')`)
	ex(`INSERT INTO doc_video(docid, pub_id, key) VALUES(101,1,'pub-inv-1_2_VIDEO')`)
	ex(`INSERT INTO video(key, lang, title, duration, subtitles, json, fetched_at) VALUES('pub-inv-1_1_VIDEO','E','An Invented Video',75,'','','')`)
	ex(`INSERT INTO cue(key, lang, seq, start_ms, end_ms, text) VALUES('pub-inv-1_1_VIDEO','E',1,0,2000,'first invented line')`)
	ex(`INSERT INTO cue(key, lang, seq, start_ms, end_ms, text) VALUES('pub-inv-1_1_VIDEO','E',2,2000,4000,'second invented line')`)
	// Two placeable citations and one the numbering cannot resolve.
	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,5,20012,20012,1)`)
	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,6,20013,20014,1)`)
	ex(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(101,7,99999999,99999999,1)`)
	return s
}
