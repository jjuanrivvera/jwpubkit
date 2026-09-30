package store

import (
	"fmt"
	"strings"
	"unicode"
)

// navigationalClasses are document classes that exist to help a reader find
// something rather than to say anything: covers, tables of contents, indexes,
// front matter, the table of Bible books. A question asked in ordinary words
// almost never wants one, and they crowd out the articles that do answer it.
// They are demoted rather than dropped, because "which index entry covers this"
// is a fair question too — it is just not the usual one.
//
// The numbers are MEPS document classes, which are the same in every language;
// the titles that name them are not.
var navigationalClasses = map[int]bool{
	12:  true, // front matter, word index
	18:  true, // cover / publishers' page
	39:  true, // cover
	68:  true, // index
	117: true, // names and order of the Bible books
}

// indexClasses are the subject-index documents: an entry per theme, whose body
// is a list of pointers into other publications. Useful, but an answer to a
// different question.
var indexClasses = map[int]bool{
	3: true,
	4: true, // the per-month container of a daily-text publication
}

// Question searches the library the way a question is asked, rather than
// demanding every word appear in one paragraph.
//
// Measured against six questions a person actually asked while preparing, the
// all-words search returned nothing for five of them: "¿Cómo consolar a alguien
// que perdió a un ser querido?" shares no single paragraph with every one of its
// words, and no publication is written to. So the words are taken as evidence
// rather than as requirements: a paragraph matching four of five counts, ranked
// by how much of the question it covers and by BM25 within that.
//
// The words that carry no evidence are dropped first, and which ones those are is
// derived from the library itself — a term in a fifth of all paragraphs tells you
// nothing — rather than from a stopword list per language, of which this tool
// would need a thousand.
func (s *Store) Question(query string, pubs []string, limit int) ([]SearchHit, error) {
	terms, dropped, err := s.signalTerms(query)
	if err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		// Every word was too common to mean anything: fall back to the exact
		// search rather than returning the whole library.
		return s.Search(query, pubs, limit)
	}
	if limit <= 0 {
		limit = 20
	}
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(t, `"`, `""`)+`"`)
	}
	fq := strings.Join(quoted, " OR ")

	args := []any{fq}
	where := ""
	if len(pubs) > 0 {
		var ors []string
		for _, p := range pubs {
			ors = append(ors, "pub.symbol = ? OR pub.meps_symbol = ?")
			args = append(args, p, p)
		}
		where = " AND (" + strings.Join(ors, " OR ") + ")"
	}
	args = append(args, limit)

	// coverage counts how many of the question's terms the document matches at
	// all, which is what separates a paragraph about the subject from one that
	// happens to share a word. The class penalty pushes navigation below prose.
	q := `WITH hits AS (
		SELECT par.docid, par.pid, par.num, bm25(par_fts) AS rank,
			snippet(par_fts, 0, '«', '»', '…', 14) AS snip
		FROM par_fts JOIN par ON par.id = par_fts.rowid
		WHERE par_fts MATCH ?
	), best AS (
		SELECT hits.*, count(*) OVER (PARTITION BY hits.docid) AS n,
			row_number() OVER (PARTITION BY hits.docid ORDER BY hits.rank) AS rn
		FROM hits
	)
	SELECT best.docid, COALESCE(pub.meps_symbol, pub.symbol), pub.key, COALESCE(doc.title,''),
		best.pid, COALESCE(best.num,0), best.snip, best.n, best.rank, COALESCE(pub.issue,''),
		COALESCE(doc.class,0)
	FROM best JOIN doc ON doc.docid = best.docid JOIN pub ON pub.id = doc.pub_id
	WHERE best.rn = 1` + where + `
	ORDER BY best.rank - ln(best.n) LIMIT ?`

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("question %q: %w", fq, err)
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		var class int
		if err := rows.Scan(&h.DocID, &h.Pub, &h.PubKey, &h.Title, &h.PID, &h.Num,
			&h.Snippet, &h.Matches, &h.Rank, &h.Issue, &class); err != nil {
			return nil, err
		}
		h.Kind = documentKind(class)
		h.URL = contentDocURL(h.DocID, h.PID)
		h.Cite = citeOf(h.Pub, h.Issue, h.Num, h.DocID, h.PID)
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortByAnswerFirst(out)
	if len(out) > limit {
		out = out[:limit]
	}
	_ = dropped
	return out, nil
}

// documentKind names what a document is for, so a caller can tell an article from
// an index entry without knowing MEPS class numbers.
func documentKind(class int) string {
	switch {
	case navigationalClasses[class]:
		return "navigation"
	case indexClasses[class]:
		return "index"
	}
	return "article"
}

// sortByAnswerFirst puts prose ahead of index entries and navigation, keeping the
// relevance order within each group. Stable, so BM25 still decides among equals.
func sortByAnswerFirst(hits []SearchHit) {
	rankOf := func(kind string) int {
		switch kind {
		case "article":
			return 0
		case "index":
			return 1
		default:
			return 2
		}
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && rankOf(hits[j].Kind) < rankOf(hits[j-1].Kind); j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}

// signalTerms splits a question into the words worth searching for, and reports
// the ones dropped for being everywhere.
//
// "Everywhere" is measured, not listed: a term matching more than a fifth of the
// library's paragraphs cannot narrow anything down. That is how the tool avoids
// carrying a stopword list for each of the languages jw.org publishes in.
func (s *Store) signalTerms(query string) (keep, dropped []string, err error) {
	var total int
	if err := s.DB.QueryRow(`SELECT count(*) FROM par`).Scan(&total); err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return nil, nil, nil
	}
	// "This term is everywhere" is a measurement, and a measurement needs enough
	// to measure. Under a few hundred paragraphs a word in a fifth of them is not
	// a stopword, it is a small library — so the rule stays out of the way until
	// there is data to support it.
	const enoughToJudge = 500
	ceiling := total
	if total >= enoughToJudge {
		ceiling = total / 5
	}

	seen := map[string]bool{}
	for _, w := range splitWords(query) {
		if seen[w] {
			continue
		}
		seen[w] = true
		var n int
		// A term FTS5 cannot parse is not worth a query.
		if err := s.DB.QueryRow(`SELECT count(*) FROM par_fts WHERE par_fts MATCH ?`, `"`+w+`"`).Scan(&n); err != nil {
			continue
		}
		if n == 0 || n > ceiling {
			dropped = append(dropped, w)
			continue
		}
		keep = append(keep, w)
	}
	return keep, dropped, nil
}

// splitWords takes the words out of a question, in any script: letters and digits
// are words, everything else separates them. Single characters are dropped, since
// no script uses them to carry a subject on their own — except the ones written
// without spaces, where a single character can be a word, so those are kept.
func splitWords(q string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		w := cur.String()
		cur.Reset()
		if w == "" {
			return
		}
		if len([]rune(w)) == 1 && !isDense(w) {
			return
		}
		out = append(out, strings.ToLower(w))
	}
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func isDense(w string) bool {
	for _, r := range w {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Thai, r) {
			return true
		}
	}
	return false
}
