package store

import (
	"fmt"
	"strings"
)

// ChainStep is one verse reached while following marginal references, with how
// it was reached.
type ChainStep struct {
	VerseID int    `json:"verse_id"`
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
	Verse   int    `json:"verse"`
	Text    string `json:"text"`
	// Hop is how many references away from the starting passage this is: 1 is a
	// verse the passage points at, 2 is a verse that one points at, and so on.
	Hop int `json:"hop"`
	// From is the verse whose margin pointed here, 0 for the starting passage.
	From int `json:"from_verse_id"`
	// Anchor is the word in the source verse the reference hangs off.
	Anchor string `json:"anchor,omitempty"`
}

// Chain follows the marginal references out of a passage, up to hops deep.
//
// It walks breadth-first and never visits a verse twice: marginal references
// form a cycle-rich graph (two verses routinely point at each other), so a naive
// walk does not terminate and a depth-first one reports a verse at a worse hop
// count than the one it is actually reachable at. limit caps the total returned
// so a wide passage cannot produce an unreadable list.
func (s *Store) Chain(firstID, lastID, hops, limit int) ([]ChainStep, error) {
	if hops < 1 {
		hops = 1
	}
	if limit <= 0 {
		limit = 200
	}
	seen := map[int]bool{}
	for id := firstID; id <= lastID; id++ {
		seen[id] = true
	}
	frontier := make([]int, 0, lastID-firstID+1)
	for id := firstID; id <= lastID; id++ {
		frontier = append(frontier, id)
	}

	var out []ChainStep
	for hop := 1; hop <= hops && len(frontier) > 0 && len(out) < limit; hop++ {
		targets, err := s.xrefTargets(frontier)
		if err != nil {
			return nil, err
		}
		var next []int
		for _, t := range targets {
			if seen[t.verse] {
				continue
			}
			seen[t.verse] = true
			v, err := s.verseAt(t.verse)
			if err != nil {
				continue // a reference into a book the library's Bible does not carry
			}
			v.Hop, v.From, v.Anchor = hop, t.from, t.anchor
			out = append(out, v)
			next = append(next, t.verse)
			if len(out) >= limit {
				break
			}
		}
		frontier = next
	}
	return out, nil
}

type xrefTarget struct {
	from, verse int
	anchor      string
}

// xrefTargets expands one hop: every verse the margins of these verses point at.
func (s *Store) xrefTargets(from []int) ([]xrefTarget, error) {
	if len(from) == 0 {
		return nil, nil
	}
	holes := strings.TrimSuffix(strings.Repeat("?,", len(from)), ",")
	args := make([]any, 0, len(from))
	for _, id := range from {
		args = append(args, id)
	}
	rows, err := s.DB.Query(`SELECT verse_id, first, last, COALESCE(anchor,'')
		FROM verse_xref WHERE verse_id IN (`+holes+`) ORDER BY verse_id, seq`, args...)
	if err != nil {
		return nil, fmt.Errorf("marginal references: %w", err)
	}
	defer rows.Close()
	var out []xrefTarget
	for rows.Next() {
		var src, first, last int
		var anchor string
		if err := rows.Scan(&src, &first, &last, &anchor); err != nil {
			return nil, err
		}
		// A reference can span a range; every verse in it is reached.
		for id := first; id <= last && id-first < 64; id++ {
			out = append(out, xrefTarget{from: src, verse: id, anchor: anchor})
		}
	}
	return out, rows.Err()
}

func (s *Store) verseAt(id int) (ChainStep, error) {
	var v ChainStep
	err := s.DB.QueryRow(`SELECT id, book, chapter, verse, text FROM verse WHERE id=?`, id).
		Scan(&v.VerseID, &v.Book, &v.Chapter, &v.Verse, &v.Text)
	return v, err
}
