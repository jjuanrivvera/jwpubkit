package store

import "testing"

// A chain has to terminate and to report the shortest way it reached a verse.
// Marginal references point both ways all the time, so a walk that does not
// remember where it has been runs forever.
func TestChainIsBreadthFirstAndTerminates(t *testing.T) {
	s, err := Open(t.TempDir(), "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// 1 -> 2 -> 3, and 2 -> 1 back again, plus a long way round 1 -> 3.
	for id, text := range map[int]string{1: "first", 2: "second", 3: "third"} {
		exec(`INSERT INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,1,1,?,?,1)`, id, id, text)
	}
	xref := func(from, to, seq int, anchor string) {
		exec(`INSERT INTO verse_xref(verse_id, mid, marker, anchor, seq, first, last, pub_id)
			VALUES(?,?,'a',?,?,?,?,1)`, from, seq, anchor, seq, to, to)
	}
	xref(1, 2, 1, "a word")
	xref(2, 3, 1, "another")
	xref(2, 1, 2, "back")
	xref(1, 3, 2, "shortcut")

	steps, err := s.Chain(1, 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("steps = %+v, want verses 2 and 3 once each", steps)
	}
	byID := map[int]ChainStep{}
	for _, st := range steps {
		byID[st.VerseID] = st
	}
	if byID[2].Hop != 1 || byID[2].From != 1 {
		t.Errorf("verse 2 = %+v, want hop 1 from verse 1", byID[2])
	}
	// Verse 3 is reachable at hop 1 through the shortcut, so that is what it is.
	if byID[3].Hop != 1 {
		t.Errorf("verse 3 = %+v, want the shortest hop", byID[3])
	}
	if byID[2].Anchor != "a word" {
		t.Errorf("the anchor should say which word pointed there: %+v", byID[2])
	}

	// The starting passage is never reported as part of its own chain.
	for _, st := range steps {
		if st.VerseID == 1 {
			t.Error("the passage itself came back in its own chain")
		}
	}
}

func TestChainRespectsItsLimits(t *testing.T) {
	s, err := Open(t.TempDir(), "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 40; id++ {
		if _, err := s.DB.Exec(`INSERT INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,1,1,?,'t',1)`, id, id); err != nil {
			t.Fatal(err)
		}
		if id > 1 {
			if _, err := s.DB.Exec(`INSERT INTO verse_xref(verse_id, mid, marker, anchor, seq, first, last, pub_id)
				VALUES(1,?,'a','w',?,?,?,1)`, id, id, id, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	steps, err := s.Chain(1, 1, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 10 {
		t.Errorf("limit ignored: %d steps", len(steps))
	}
	// A zero hop count still does one hop rather than nothing.
	if steps, _ := s.Chain(1, 1, 0, 100); len(steps) == 0 {
		t.Error("hops<1 should still follow one hop")
	}
}
