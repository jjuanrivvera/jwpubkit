package store

import (
	"fmt"
	"strings"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
)

// Edge is one connection the library already records between two things.
//
// The graph is not inferred and not embedded: publications state these links
// themselves — a citation, a marginal reference, an extract of one work inside
// another, a video embedded in an article. What was missing was a way to walk
// them. Every edge therefore carries the signal it came from, so an answer can
// be checked rather than trusted.
type Edge struct {
	// Relation is what the edge means, from the subject's point of view.
	Relation string `json:"relation"`
	// Source names the table or markup the edge was read from.
	Source string `json:"source"`

	DocID     int    `json:"docid,omitempty"`
	Pub       string `json:"publication,omitempty"`
	Title     string `json:"title,omitempty"`
	Cite      string `json:"cite,omitempty"`
	Paragraph int    `json:"paragraph,omitempty"`
	URL       string `json:"url,omitempty"`
	Kind      string `json:"document_kind,omitempty"`

	// Reference is set when the other end is a Bible passage.
	Reference string `json:"reference,omitempty"`
	// VideoKey is set when the other end is a video.
	VideoKey string `json:"video_key,omitempty"`
	// Weight is how often the connection occurs, for co-citation.
	Weight int `json:"weight,omitempty"`
	// Term is set when the other end is a defined term.
	Term string `json:"term,omitempty"`
	// Text is a short piece of the other end, when there is one worth showing.
	Text string `json:"text,omitempty"`
}

// GraphLimits caps what a walk returns. A verse like John 3:16 is cited by
// hundreds of documents; without caps the answer is unreadable and the JSON is
// megabytes.
type GraphLimits struct {
	PerRelation int
	CoCitation  int
}

func (g GraphLimits) perRelation() int {
	if g.PerRelation <= 0 {
		return 25
	}
	return g.PerRelation
}

func (g GraphLimits) coCitation() int {
	if g.CoCitation <= 0 {
		return 15
	}
	return g.CoCitation
}

// VerseGraph collects everything the library records about a passage.
func (s *Store) VerseGraph(first, last int, lim GraphLimits) ([]Edge, error) {
	var out []Edge
	add := func(e ...Edge) { out = append(out, e...) }

	cited, err := s.citingDocuments(first, last, lim.perRelation())
	if err != nil {
		return nil, err
	}
	add(cited...)

	xrefs, err := s.marginalEdges(first, last, lim.perRelation())
	if err != nil {
		return nil, err
	}
	add(xrefs...)

	co, err := s.coCited(first, last, lim.coCitation())
	if err != nil {
		return nil, err
	}
	add(co...)

	vids, err := s.videosCiting(first, last, lim.perRelation())
	if err != nil {
		return nil, err
	}
	add(vids...)

	terms, err := s.definedTerms(first, last, lim.perRelation())
	if err != nil {
		return nil, err
	}
	add(terms...)
	return out, nil
}

// citingDocuments is the plainest edge there is: a publication quoting a verse,
// recorded by the publication itself.
func (s *Store) citingDocuments(first, last, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT c.docid, COALESCE(p.meps_symbol, p.symbol),
			COALESCE(d.title,''), COALESCE(c.pid,0), COALESCE(p.issue,''), COALESCE(d.class,0)
		FROM cite c JOIN doc d ON d.docid = c.docid JOIN pub p ON p.id = d.pub_id
		WHERE c.first <= ? AND c.last >= ?
		ORDER BY COALESCE(p.year,0) DESC, c.docid LIMIT ?`, last, first, limit)
	if err != nil {
		return nil, fmt.Errorf("citing documents: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		e := Edge{Relation: "cited-by", Source: "cite (BibleCitation)"}
		var issue string
		var class int
		if err := rows.Scan(&e.DocID, &e.Pub, &e.Title, &e.Paragraph, &issue, &class); err != nil {
			return nil, err
		}
		e.Kind = documentKind(class)
		e.Cite = citeOf(e.Pub, issue, e.Paragraph, e.DocID, e.Paragraph)
		e.URL = contentDocURL(e.DocID, e.Paragraph)
		out = append(out, e)
	}
	return out, rows.Err()
}

// marginalEdges are the references printed in the Bible's own margin.
func (s *Store) marginalEdges(first, last, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT first, last, COALESCE(anchor,'')
		FROM verse_xref WHERE verse_id BETWEEN ? AND ? ORDER BY first LIMIT ?`, first, last, limit)
	if err != nil {
		return nil, fmt.Errorf("marginal references: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		var f, l int
		var anchor string
		if err := rows.Scan(&f, &l, &anchor); err != nil {
			return nil, err
		}
		e := Edge{Relation: "points-to", Source: "verse_xref (marginal reference)", Text: anchor}
		if r, ok := bible.FromIDs(f, l); ok {
			e.Reference = r.String()
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// coCited are the passages that publications quote in the same paragraph as this
// one. It is the most useful edge nobody writes down: two verses cited together
// forty times are being taught together.
func (s *Store) coCited(first, last, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT o.first, o.last, count(*) AS weight
		FROM cite c JOIN cite o ON o.docid = c.docid AND o.pid = c.pid
		WHERE c.first <= ? AND c.last >= ? AND NOT (o.first <= ? AND o.last >= ?)
		GROUP BY o.first, o.last ORDER BY weight DESC, o.first LIMIT ?`,
		last, first, last, first, limit)
	if err != nil {
		return nil, fmt.Errorf("co-citation: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		var f, l, w int
		if err := rows.Scan(&f, &l, &w); err != nil {
			return nil, err
		}
		e := Edge{Relation: "cited-alongside", Source: "cite ∩ cite, same paragraph", Weight: w}
		if r, ok := bible.FromIDs(f, l); ok {
			e.Reference = r.String()
		}
		if e.Reference == "" {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// videosCiting finds the videos embedded in documents that quote the passage.
func (s *Store) videosCiting(first, last, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT dv.key, dv.docid, COALESCE(d.title,''),
			COALESCE(v.title,'')
		FROM cite c JOIN doc_video dv ON dv.docid = c.docid
		JOIN doc d ON d.docid = dv.docid
		LEFT JOIN video v ON v.key = dv.key AND v.lang = ?
		WHERE c.first <= ? AND c.last >= ? LIMIT ?`, s.Lang, last, first, limit)
	if err != nil {
		return nil, fmt.Errorf("videos: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		e := Edge{Relation: "video-in-citing-document", Source: "doc_video (document markup)"}
		var vtitle string
		if err := rows.Scan(&e.VideoKey, &e.DocID, &e.Title, &vtitle); err != nil {
			return nil, err
		}
		if vtitle != "" {
			e.Text = vtitle
		}
		e.URL = content.FinderURL(e.VideoKey)
		out = append(out, e)
	}
	return out, rows.Err()
}

// definedTerms are the glossary entries the passage's study notes point at.
func (s *Store) definedTerms(first, last, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT COALESCE(html,'') FROM verse_note
		WHERE verse_id BETWEEN ? AND ? LIMIT ?`, first, last, limit)
	if err != nil {
		return nil, err
	}
	var htmls []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return nil, err
		}
		htmls = append(htmls, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []Edge
	seen := map[int]bool{}
	for _, h := range htmls {
		for _, d := range definitionsIn(h) {
			if seen[d.DocID] {
				continue
			}
			seen[d.DocID] = true
			e := Edge{Relation: "defines-term", Source: "study note link (class xt)",
				Term: d.Term, DocID: d.DocID, URL: d.URL}
			if def, ok := s.Define(d.Term); ok {
				e.Text = def.Text
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// DocGraph collects everything the library records about a document.
func (s *Store) DocGraph(docid int, lim GraphLimits) ([]Edge, error) {
	var out []Edge

	// What quotes this document, and what this document quotes.
	rows, err := s.DB.Query(`SELECT e.docid, COALESCE(p.meps_symbol, p.symbol), COALESCE(d.title,''),
			COALESCE(e.begin_pid,0), COALESCE(e.caption,''), COALESCE(p.issue,''), COALESCE(d.class,0)
		FROM extract e JOIN doc d ON d.docid = e.docid JOIN pub p ON p.id = e.pub_id
		WHERE e.ref_docid = ? LIMIT ?`, docid, lim.perRelation())
	if err != nil {
		return nil, fmt.Errorf("extracts of this document: %w", err)
	}
	for rows.Next() {
		e := Edge{Relation: "extracted-by", Source: "extract (DocumentExtract)"}
		var issue, caption string
		var class int
		if err := rows.Scan(&e.DocID, &e.Pub, &e.Title, &e.Paragraph, &caption, &issue, &class); err != nil {
			rows.Close()
			return nil, err
		}
		e.Kind = documentKind(class)
		e.Cite = content.Cite(caption, e.Pub, issue, 0, 0, e.DocID, e.Paragraph).Text
		e.URL = contentDocURL(e.DocID, e.Paragraph)
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	refs, err := s.docReferences(docid, lim.perRelation())
	if err != nil {
		return nil, err
	}
	out = append(out, refs...)

	vids, err := s.docVideos(docid, lim.perRelation())
	if err != nil {
		return nil, err
	}
	out = append(out, vids...)

	verses, err := s.docVerses(docid, lim.perRelation())
	if err != nil {
		return nil, err
	}
	return append(out, verses...), nil
}

// docReferences are the publications this document points at, taken from the
// extracts it carries rather than from its prose.
func (s *Store) docReferences(docid, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT e.ref_docid, COALESCE(e.ref_symbol,''), COALESCE(e.title,''),
			COALESCE(e.caption,''), COALESCE(e.ref_class,0)
		FROM extract e WHERE e.docid = ? AND e.ref_docid <> 0 LIMIT ?`, docid, limit)
	if err != nil {
		return nil, fmt.Errorf("references of this document: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		e := Edge{Relation: "refers-to", Source: "extract (DocumentExtract)"}
		var caption string
		var class int
		if err := rows.Scan(&e.DocID, &e.Pub, &e.Title, &caption, &class); err != nil {
			return nil, err
		}
		e.Kind = documentKind(class)
		e.Cite = content.Cite(caption, e.Pub, "", 0, 0, e.DocID, 0).Text
		e.URL = contentDocURL(e.DocID, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) docVideos(docid, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT dv.key, COALESCE(v.title,''),
			(SELECT count(*) FROM cue WHERE cue.key = dv.key AND cue.lang = ?) AS cues
		FROM doc_video dv LEFT JOIN video v ON v.key = dv.key AND v.lang = ?
		WHERE dv.docid = ? LIMIT ?`, s.Lang, s.Lang, docid, limit)
	if err != nil {
		return nil, fmt.Errorf("videos of this document: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		e := Edge{Relation: "embeds-video", Source: "doc_video (document markup)"}
		var cues int
		if err := rows.Scan(&e.VideoKey, &e.Text, &cues); err != nil {
			return nil, err
		}
		e.Weight = cues
		e.URL = content.FinderURL(e.VideoKey)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) docVerses(docid, limit int) ([]Edge, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT first, last FROM cite WHERE docid = ? ORDER BY first LIMIT ?`,
		docid, limit)
	if err != nil {
		return nil, fmt.Errorf("verses of this document: %w", err)
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		var f, l int
		if err := rows.Scan(&f, &l); err != nil {
			return nil, err
		}
		e := Edge{Relation: "quotes", Source: "cite (BibleCitation)"}
		if r, ok := bible.FromIDs(f, l); ok {
			e.Reference = r.String()
		}
		if e.Reference == "" {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// VideoDocuments lists the documents that embed a video, which is what turns a
// transcript hit into "and here is where this video is used".
func (s *Store) VideoDocuments(key string, limit int) ([]Edge, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.DB.Query(`SELECT dv.docid, COALESCE(p.meps_symbol, p.symbol), COALESCE(d.title,''),
			COALESCE(p.issue,''), COALESCE(d.class,0)
		FROM doc_video dv JOIN doc d ON d.docid = dv.docid JOIN pub p ON p.id = d.pub_id
		WHERE dv.key = ? ORDER BY COALESCE(p.year,0) DESC LIMIT ?`, key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		e := Edge{Relation: "used-in", Source: "doc_video (document markup)"}
		var issue string
		var class int
		if err := rows.Scan(&e.DocID, &e.Pub, &e.Title, &issue, &class); err != nil {
			return nil, err
		}
		e.Kind = documentKind(class)
		e.Cite = citeOf(e.Pub, issue, 0, e.DocID, 0)
		e.URL = contentDocURL(e.DocID, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

var _ = strings.TrimSpace
