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
