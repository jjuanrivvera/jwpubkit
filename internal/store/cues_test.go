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
