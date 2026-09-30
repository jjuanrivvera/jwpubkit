package store

import (
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// A glossary is indexed from a document's markup, and the markup says which
// part is the term and which the definition through untranslated class names
// (de, dt, dd). A publication in any language has to split the same way.
func TestIndexGlossarySplitsTermFromDefinition(t *testing.T) {
	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	pub := testutil.Pub{
		Symbol: "nwtsty", Undated: "nwtsty", Year: 2026, Title: "Invented Study Bible",
		Docs: []testutil.Doc{{ID: 1, MepsID: 2026500, Class: glossaryClass, Title: "Invented Glossary",
			HTML: `<div id="p1" data-pid="1" class="de"><p class="dt">Aardwolf</p>` +
				`<p class="dd">An invented animal of the invented plain.</p></div>` +
				`<div id="p2" data-pid="2" class="de"><p class="dt">Bramblewort, Bramble-wort</p>` +
				`<p class="dd">An invented plant, written two ways.</p></div>` +
				`<p id="p3" data-pid="3">An ordinary paragraph, which defines nothing.</p>`}},
	}
	if _, err := s.IndexLocal(testutil.Build(t, t.TempDir(), pub), "nwtsty", "", "E"); err != nil {
		t.Fatal(err)
	}

	// Two entries. The second spells its term two ways, but both fold to the
	// same key, so they are one way in — displayed under the principal
	// spelling, which is the one the publication writes first.
	if n := s.GlossarySize(); n != 2 {
		t.Errorf("GlossarySize = %d, want 2", n)
	}
	terms, err := s.Terms("", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 2 {
		t.Fatalf("Terms = %+v", terms)
	}

	byTerm := map[string]GlossaryEntry{}
	for _, e := range terms {
		byTerm[e.Term] = e
	}
	entry, ok := byTerm["Aardwolf"]
	if !ok {
		t.Fatalf("the first term is missing: %+v", terms)
	}
	if !strings.Contains(entry.Text, "invented animal") {
		t.Errorf("the definition should be the dd, not the term: %q", entry.Text)
	}
	if strings.Contains(entry.Text, "Aardwolf") {
		t.Errorf("the term should not be swallowed into its own definition: %q", entry.Text)
	}
	if entry.DocID != 2026500 || entry.PID != 1 {
		t.Errorf("an entry should point at where it is: %+v", entry)
	}

	// The principal spelling is what is shown, not the variant that happened to
	// be written last.
	if _, ok := byTerm["Bramble-wort"]; ok {
		t.Errorf("the variant spelling overwrote the principal one: %+v", terms)
	}
	shown, ok := byTerm["Bramblewort"]
	if !ok {
		t.Fatalf("the principal spelling is missing: %+v", terms)
	}
	if !strings.Contains(shown.Text, "invented plant") {
		t.Errorf("Bramblewort = %q", shown.Text)
	}
	// Either spelling still finds it, because a lookup folds the same way.
	for _, typed := range []string{"Bramblewort", "Bramble-wort", "bramblewort"} {
		found, err := s.Terms(typed, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].Term != "Bramblewort" {
			t.Errorf("looking up %q found %+v", typed, found)
		}
	}

	// A paragraph that defines nothing is not an entry.
	for _, e := range terms {
		if strings.Contains(e.Text, "ordinary paragraph") {
			t.Errorf("an ordinary paragraph became a glossary entry: %+v", e)
		}
	}
}

// A document that is not a glossary must not be mined for entries: the class is
// what says a document defines terms.
func TestOnlyAGlossaryDocumentIsIndexedAsOne(t *testing.T) {
	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	pub := testutil.Pub{
		Symbol: "w26.01", Undated: "w", Year: 2026, Title: "Invented Magazine",
		Docs: []testutil.Doc{{ID: 1, MepsID: 2026600, Class: 40, Title: "An Invented Article",
			HTML: `<div id="p1" data-pid="1" class="de"><p class="dt">Aardwolf</p>` +
				`<p class="dd">An invented animal.</p></div>`}},
	}
	if _, err := s.IndexLocal(testutil.Build(t, t.TempDir(), pub), "w26.01", "202601", "E"); err != nil {
		t.Fatal(err)
	}
	if n := s.GlossarySize(); n != 0 {
		t.Errorf("a class-40 article defined %d terms; only class %d is a glossary", n, glossaryClass)
	}
}

// The index summary is interface, so it is in English and only mentions what it
// counted: a publication with no verses should not report zero of four things.
func TestIndexStatsStringSaysOnlyWhatItCounted(t *testing.T) {
	plain := IndexStats{Docs: 2, Pars: 40, Cites: 7, Media: 3, Extracts: 1}.String()
	for _, absent := range []string{"verses", "study notes", "weeks", "semanas"} {
		if strings.Contains(plain, absent) {
			t.Errorf("a publication with none of them should not mention %q: %q", absent, plain)
		}
	}
	if !strings.Contains(plain, "2 documents") || !strings.Contains(plain, "7 bible citations") {
		t.Errorf("summary = %q", plain)
	}

	full := IndexStats{Docs: 1, Pars: 10, Verses: 31, Notes: 4, Footnotes: 2, XRefs: 9, Dated: 52}.String()
	for _, want := range []string{"31 verses", "4 study notes", "9 marginal references", "52 weeks"} {
		if !strings.Contains(full, want) {
			t.Errorf("summary should contain %q: %q", want, full)
		}
	}
	if strings.Contains(full, "semanas") {
		t.Errorf("the summary is interface, and the interface is English: %q", full)
	}
}
