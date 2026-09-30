package store

import (
	"fmt"

	"github.com/jjuanrivvera/jwpubkit/internal/content"
)

// Commented is one publication that comments on a verse, with the paragraphs it
// does so in and the text it says there.
//
// This is the richest edge in the library and it was invisible. A study guide
// indexes a verse by pointing at a BLOCK of its own index document, and the real
// pointers — to a hundred and thirty publications, most of which nobody has
// synced — are the extracts inside that block, each carrying the quoted paragraph.
// Joining the two needs the block's paragraph range, which was being read from the
// publication and then dropped on the floor.
type Commented struct {
	Pub       string `json:"publication"`
	DocID     int    `json:"docid"`
	Title     string `json:"title,omitempty"`
	Cite      string `json:"cite"`
	URL       string `json:"url"`
	Paragraph int    `json:"paragraph,omitempty"`
	// Text is the paragraph as the citing publication wrote it, which ships inside
	// the index rather than requiring that publication to be synced.
	Text string `json:"text,omitempty"`
	// InLibrary says whether the citing publication is synced, so a reader knows
	// whether there is more where this came from.
	InLibrary bool `json:"in_library"`
	// Via names the index that recorded the pointer.
	Via string `json:"via"`
}

// CommentedOn lists the publications that comment on a verse, through whatever
// index in the library records such things.
//
// It is additive to the citation table, not a replacement: a citation says "this
// paragraph mentions the verse", which for an index document is its own heading
// and useless on its own. This says "this publication comments on the verse, here,
// and here is what it says".
func (s *Store) CommentedOn(verseID, limit int) ([]Commented, error) {
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.DB.Query(`
		SELECT COALESCE(e.ref_undated, e.ref_symbol, ''), e.ref_docid, COALESCE(e.title,''),
			COALESCE(e.caption,''), COALESCE(e.ref_begin,0), COALESCE(e.html,''),
			COALESCE(ip.symbol,''), COALESCE(rp.title,'')
		FROM verse_note n
		JOIN extract e ON e.docid = n.docid AND e.pub_id = n.pub_id
			AND e.begin_pid >= n.begin_pid AND e.begin_pid <= n.end_pid
		JOIN pub ip ON ip.id = n.pub_id
		LEFT JOIN doc rd ON rd.docid = e.ref_docid
		LEFT JOIN pub rp ON rp.id = rd.pub_id
		WHERE n.verse_id = ? AND n.begin_pid IS NOT NULL AND n.end_pid IS NOT NULL
			AND e.ref_docid <> 0
		ORDER BY e.begin_pid, e.sort LIMIT ?`, verseID, limit)
	if err != nil {
		return nil, fmt.Errorf("commentary on verse %d: %w", verseID, err)
	}
	defer rows.Close()
	var out []Commented
	seen := map[[2]int]bool{}
	for rows.Next() {
		var c Commented
		var caption, html, indexPub, refPubTitle string
		if err := rows.Scan(&c.Pub, &c.DocID, &c.Title, &caption, &c.Paragraph, &html,
			&indexPub, &refPubTitle); err != nil {
			return nil, err
		}
		key := [2]int{c.DocID, c.Paragraph}
		if seen[key] {
			continue
		}
		seen[key] = true
		c.Via = indexPub
		c.InLibrary = refPubTitle != ""
		c.Cite = content.Cite(caption, c.Pub, "", 0, 0, c.DocID, c.Paragraph).Text
		c.URL = content.DocURL(c.DocID, c.Paragraph)
		c.Text = content.InnerText(html)
		out = append(out, c)
	}
	return out, rows.Err()
}

// HasCommentaryIndex reports whether any publication in the library indexes verses
// by paragraph range, which is what makes CommentedOn able to answer anything.
func (s *Store) HasCommentaryIndex() bool {
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM verse_note
		WHERE begin_pid IS NOT NULL AND end_pid IS NOT NULL`).Scan(&n)
	return err == nil && n > 0
}
