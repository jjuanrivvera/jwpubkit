package store

import (
	"strings"
	"testing"
)

// The words that narrow a search down are decided by the library, not by a list
// per language: a term in a fifth of the paragraphs cannot tell anything apart.
// The rule needs enough paragraphs to mean anything, so this fixture has them.
func TestSignalTermsDropWhatIsEverywhere(t *testing.T) {
	s, err := Open(t.TempDir(), "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO pub(id, key, symbol, issue, lang, title) VALUES(1,'w_S','w','','S','Revista')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(101,1,1,40,'Artículo','')`); err != nil {
		t.Fatal(err)
	}
	ins, err := s.DB.Prepare(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(101,?,?,0,'p',?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer ins.Close()
	// 600 paragraphs, every one containing "de"; one containing "afligido".
	for i := 1; i <= 600; i++ {
		text := "una frase de prueba"
		if i == 7 {
			text = "un hermano afligido necesita consuelo de verdad"
		}
		if _, err := ins.Exec(i, i, text); err != nil {
			t.Fatal(err)
		}
	}

	keep, dropped, err := s.signalTerms("consuelo de afligido")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(keep, "afligido") || !contains(keep, "consuelo") {
		t.Errorf("the rare words should be kept: keep=%v dropped=%v", keep, dropped)
	}
	if !contains(dropped, "de") {
		t.Errorf("a word in all 600 paragraphs should be dropped: %v", dropped)
	}
	// A word the library has never seen makes an OR search no wider.
	if _, drop, _ := s.signalTerms("afligido aardvark"); !contains(drop, "aardvark") {
		t.Errorf("an unknown word should be dropped: %v", drop)
	}
}

// On a library too small to judge, the rule stays out of the way rather than
// dropping everything: a word in two of three paragraphs is not a stopword, it is
// a small library. Getting this wrong made the search return nothing at all.
func TestSignalTermsNeedEnoughDataToJudge(t *testing.T) {
	s := questionFixture(t)
	defer s.Close()
	keep, dropped, err := s.signalTerms("consolar de")
	if err != nil {
		t.Fatal(err)
	}
	if len(keep) != 2 {
		t.Errorf("nothing should be dropped from a tiny library: keep=%v dropped=%v", keep, dropped)
	}
}

// The question the all-words search could not answer.
func TestQuestionFindsWhatAllWordsCannot(t *testing.T) {
	s := questionFixture(t)
	defer s.Close()

	q := "¿Cómo consolar a alguien que perdió a un ser querido?"
	strict, err := s.Search(q, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(strict) != 0 {
		t.Fatalf("the fixture is meant to defeat the all-words search, got %d hits", len(strict))
	}
	loose, err := s.Question(q, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(loose) == 0 {
		t.Fatal("the question search returned nothing")
	}
	// Prose first, index entries and covers after, however BM25 ranked them.
	if loose[0].Kind != "article" {
		t.Errorf("an article should lead, got %+v", loose[0])
	}
	var sawIndex bool
	for i, h := range loose {
		if h.Kind == "index" {
			sawIndex = true
		}
		if h.Kind == "article" && sawIndex {
			t.Errorf("hit %d is an article after an index entry: %+v", i, loose)
		}
	}
	// Every hit is citable and openable.
	for _, h := range loose {
		if h.Cite == "" || h.URL == "" {
			t.Errorf("hit without a citation or address: %+v", h)
		}
	}
}

// With nothing but common words left, the question falls back rather than
// returning the library.
func TestQuestionFallsBackWhenNothingNarrows(t *testing.T) {
	s := questionFixture(t)
	defer s.Close()
	hits, err := s.Question("de de de", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 3 {
		t.Errorf("a question of nothing but common words returned %d hits", len(hits))
	}
}

func questionFixture(t *testing.T) *Store {
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
	exec(`INSERT INTO pub(id, key, symbol, issue, lang, meps_symbol, title, year) VALUES(2,'rsg19_S','rsg19','','S','rsg19','Guía',2019)`)
	// An article that answers the question without containing all its words.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(101,1,1,40,'El consuelo de las Escrituras','')`)
	exec(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(101,1,1,0,'p','Un hermano afligido necesita consuelo de verdad; hay que consolar de corazón.')`)
	// An index entry that shares a word.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(201,2,1,3,'El vivir cristiano','')`)
	exec(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(201,1,1,0,'p','Índice de temas: consolar, animar, ayudar, de todo.')`)
	// A cover, which should rank last of all.
	exec(`INSERT INTO doc(docid, pub_id, local_id, class, title, html) VALUES(301,1,2,39,'Portada','')`)
	exec(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(301,1,1,0,'p','Consuelo de parte de Dios.')`)
	return s
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if strings.EqualFold(v, x) {
			return true
		}
	}
	return false
}
