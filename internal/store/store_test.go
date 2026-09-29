package store

import (
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func indexPub(t *testing.T, s *Store, symbol, issue string, p testutil.Pub) *IndexStats {
	t.Helper()
	path := testutil.Build(t, t.TempDir(), p)
	st, err := s.IndexLocal(path, symbol, issue, "S")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

var encyclopedia = testutil.Pub{
	Symbol: "it", Undated: "it", Year: 2020, Title: "Perspicacia de prueba",
	Docs: []testutil.Doc{
		{ID: 0, MepsID: 1200000001, Class: 2, Title: "Cisterna",
			HTML: `<h1 id="p1" data-pid="1"><strong>CISTERNA</strong></h1>
<p id="p2" data-pid="2" class="sb"><span class="parNum" data-pnum="1"></span>Depósito excavado para almacenar agua de lluvia (<a href="jwpub://b/NWTR/24:38:6-24:38:6" class="b">Jer 38:6</a>).</p>
<p id="p3" data-pid="3" class="sb"><span class="parNum" data-pnum="2"></span>Una cisterna vacía tenía fango en el fondo.</p>`},
		{ID: 1, MepsID: 1200000002, Class: 2, Title: "Ébed-mélec",
			HTML: `<h1 id="p1" data-pid="1"><strong>ÉBED-MÉLEC</strong></h1>
<p id="p2" data-pid="2" class="sb"><span class="parNum" data-pnum="1"></span>Eunuco etíope que rescató a Jeremías (<a href="jwpub://b/NWTR/24:38:7-24:38:13" class="b">Jer 38:7-13</a>).</p>`},
	},
}

func TestIndexAndSearch(t *testing.T) {
	s := openTemp(t)
	st := indexPub(t, s, "it", "", encyclopedia)
	if st.Docs != 2 || st.Pars != 5 || st.Cites != 2 {
		t.Fatalf("stats %+v", st)
	}

	// Accents do not matter; every word must be in the same paragraph.
	hits, err := s.Search("ebed melec", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].DocID != 1200000002 || hits[0].Title != "Ébed-mélec" {
		t.Fatalf("hits %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "«ÉBED»") {
		t.Errorf("snippet %q", hits[0].Snippet)
	}

	hits, _ = s.Search("cisterna fango", nil, 10)
	if len(hits) != 1 || hits[0].PID != 3 || hits[0].Num != 2 {
		t.Fatalf("AND search %+v", hits)
	}
	hits, _ = s.Search(`"agua de lluvia"`, nil, 10)
	if len(hits) != 1 || hits[0].PID != 2 {
		t.Fatalf("phrase search %+v", hits)
	}
	hits, _ = s.Search(`"lluvia de agua"`, nil, 10)
	if len(hits) != 0 {
		t.Fatalf("phrase must keep order: %+v", hits)
	}
	hits, _ = s.Search("cistern*", nil, 10)
	if len(hits) != 1 {
		t.Fatalf("prefix search %+v", hits)
	}
	if hits, _ = s.Search("cisterna", []string{"w"}, 10); len(hits) != 0 {
		t.Fatalf("pub filter ignored: %+v", hits)
	}
	if hits, _ = s.Search("cisterna", []string{"it"}, 10); len(hits) != 1 {
		t.Fatalf("pub filter: %+v", hits)
	}
	if _, err := s.Search("  ¿? ", nil, 10); err == nil {
		t.Fatal("empty query must fail")
	}

	// Citations come from the links, in NWTR numbering.
	r, _ := bible.Parse("Jer 38:6")
	cites, total, err := s.CitedBy(r[0].FirstID(), r[0].LastID(), 0)
	if err != nil || total != 1 || cites[0].DocID != 1200000001 || cites[0].PIDs[0] != 2 {
		t.Fatalf("CitedBy Jer 38:6 = %+v %d %v", cites, total, err)
	}
	r, _ = bible.Parse("Jer 38:10")
	if cites, _, _ := s.CitedBy(r[0].FirstID(), r[0].LastID(), 0); len(cites) != 1 || cites[0].DocID != 1200000002 {
		t.Fatalf("range citation not found: %+v", cites)
	}
}

func TestReindexReplaces(t *testing.T) {
	s := openTemp(t)
	indexPub(t, s, "it", "", encyclopedia)
	changed := encyclopedia
	changed.Docs = []testutil.Doc{{ID: 0, MepsID: 1200000009, Class: 2, Title: "Otro",
		HTML: `<p id="p1" data-pid="1" class="sb">Texto nuevo sin la palabra buscada.</p>`}}
	indexPub(t, s, "it", "", changed)
	if hits, _ := s.Search("cisterna", nil, 10); len(hits) != 0 {
		t.Fatalf("stale rows after re-index: %+v", hits)
	}
	if hits, _ := s.Search("nuevo", nil, 10); len(hits) != 1 {
		t.Fatalf("new rows missing: %+v", hits)
	}
	pubs, _ := s.Pubs()
	if len(pubs) != 1 || pubs[0].Docs != 1 {
		t.Fatalf("pubs %+v", pubs)
	}
}

func TestBibleIndex(t *testing.T) {
	s := openTemp(t)
	id, _ := bible.VerseID(24, 38, 6)
	p := testutil.Pub{Symbol: "nwtsty", Undated: "nwtsty", Year: 2026, Title: "Biblia de prueba",
		Verses: map[int]string{id: `<span id="v24-38-6" class="v">Texto sintético del versículo de prueba.</span>`}}
	st := indexPub(t, s, "nwtsty", "", p)
	if st.Verses != 1 || !s.HasBible() {
		t.Fatalf("stats %+v", st)
	}
	vs, err := s.Verses(id, id)
	if err != nil || len(vs) != 1 || vs[0].Chapter != 38 || vs[0].Verse != 6 || !strings.HasPrefix(vs[0].Text, "Texto sintético") {
		t.Fatalf("verses %+v %v", vs, err)
	}
	vh, err := s.SearchVerses("sintético versículo", 5)
	if err != nil || len(vh) != 1 || vh[0].ID != id {
		t.Fatalf("verse search %+v %v", vh, err)
	}
}

func TestFTSQuery(t *testing.T) {
	cases := map[string]string{
		"Ébed-Mélec":               `"Ébed" "Mélec"`,
		`"conocimiento exacto" fe`: `"conocimiento exacto" "fe"`,
		"cistern* agua":            `"cistern"* "agua"`,
		"¿Quién me tocó?":          `"Quién" "me" "tocó"`,
		`a OR b NOT c`:             `"a" "OR" "b" "NOT" "c"`,
		`"sin cerrar`:              `"sin" "cerrar"`,
	}
	for in, want := range cases {
		if got := FTSQuery(in); got != want {
			t.Errorf("FTSQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeIssue(t *testing.T) {
	for in, want := range map[string]string{"202609": "202609", "20260900": "202609", "2026-09": "202609", "20130115": "20130115", "": ""} {
		if got := NormalizeIssue(in); got != want {
			t.Errorf("NormalizeIssue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCaption(t *testing.T) {
	loc, title := SplitCaption(`<span class="eloc">w13 15/1 pág. 9</span> <span class="etitle">Sea valiente</span>`)
	if loc != "w13 15/1 pág. 9" || title != "Sea valiente" {
		t.Fatalf("%q %q", loc, title)
	}
}
