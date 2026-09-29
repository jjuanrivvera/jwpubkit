package store

import (
	"testing"
	"time"
)

func TestCueSearchGivesTheSecond(t *testing.T) {
	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	starts := []time.Duration{2 * time.Second, 65 * time.Second, 3730 * time.Second}
	ends := []time.Duration{4 * time.Second, 68 * time.Second, 3733 * time.Second}
	texts := []string{"An invented opening line.", "The phrase we are looking for.", "A closing line."}
	if err := s.PutCues("pub-test_1_VIDEO", "E", starts, ends, texts); err != nil {
		t.Fatal(err)
	}

	hits, err := s.SearchCues("phrase looking", "E", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %+v, want exactly the one cue", hits)
	}
	if hits[0].Seconds != 65 {
		t.Errorf("Seconds = %d, want the second the cue starts at", hits[0].Seconds)
	}
	if hits[0].Key != "pub-test_1_VIDEO" {
		t.Errorf("Key = %q", hits[0].Key)
	}
	if hits[0].Snippet == "" {
		t.Error("a hit should carry a snippet")
	}

	// A phrase in no transcript finds nothing rather than something near it.
	if hits, err := s.SearchCues("aardvark", "E", 10); err != nil || len(hits) != 0 {
		t.Errorf("SearchCues for an absent word = %+v %v", hits, err)
	}

	// Another language's transcripts are not searched unless asked for.
	if hits, _ := s.SearchCues("phrase", "S", 10); len(hits) != 0 {
		t.Errorf("a different language should not match: %+v", hits)
	}
	if hits, _ := s.SearchCues("phrase", "", 10); len(hits) != 1 {
		t.Errorf("no language filter should search everything: %+v", hits)
	}
}

// Re-fetching a transcript replaces it instead of doubling it, and the FTS index
// must not keep the old rows either.
func TestPutCuesReplaces(t *testing.T) {
	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	put := func(text string) {
		if err := s.PutCues("k", "E", []time.Duration{time.Second}, []time.Duration{2 * time.Second}, []string{text}); err != nil {
			t.Fatal(err)
		}
	}
	put("first version of the line")
	put("second version of the line")

	if hits, _ := s.SearchCues("version", "E", 10); len(hits) != 1 {
		t.Errorf("a replaced transcript left %d cues, want 1", len(hits))
	}
	if hits, _ := s.SearchCues("first", "E", 10); len(hits) != 0 {
		t.Errorf("the old text is still searchable: %+v", hits)
	}
	n, err := s.CueVideoCount("E")
	if err != nil || n != 1 {
		t.Errorf("CueVideoCount = %d, %v", n, err)
	}
	if !s.HasCues("k", "E") {
		t.Error("HasCues should see the transcript")
	}
}

// A study note that says "see Glossary, X" says it with a link carrying a
// document id. Following the link is language-proof; matching the words is not.
func TestDefinitionsComeFromLinksNotProse(t *testing.T) {
	html := `<p>Una nota con una remisión (ver glosario, ` +
		`<a class="xt" data-xtid="49" href="jwpub://p/S:1001077253/"><em>arrepentimiento</em></a>` +
		`) y otra (<a class="xt" href="jwpub://p/S:1001077100/">fe</a>).</p>` +
		`<a class="b" href="jwpub://b/NWTR/24:38:6-24:38:6">Jer 38:6</a>` +
		`<a class="xt" href="jwpub://p/S:1001077253/">arrepentimiento</a>`

	got := definitionsIn(html)
	if len(got) != 2 {
		t.Fatalf("definitions = %+v, want the two distinct entries", got)
	}
	if got[0].Term != "arrepentimiento" || got[0].DocID != 1001077253 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Term != "fe" || got[1].DocID != 1001077100 {
		t.Errorf("second = %+v", got[1])
	}
	if got[0].URL == "" {
		t.Error("a definition should be openable")
	}
	// A Bible link is not a dictionary entry, and a repeat is not a second one.
	for _, d := range got {
		if d.DocID == 0 {
			t.Errorf("a link without a document id got through: %+v", d)
		}
	}

	// The same shape in another language, where only the language token differs.
	ja := `<a class="xt" href="jwpub://p/J:1001077253/">悔い改め</a>`
	if got := definitionsIn(ja); len(got) != 1 || got[0].DocID != 1001077253 {
		t.Errorf("a japanese note should resolve the same document: %+v", got)
	}
	if definitionsIn("") != nil {
		t.Error("no html, no definitions")
	}
}
