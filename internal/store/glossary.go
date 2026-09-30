package store

import (
	"strings"
	"unicode"

	"github.com/jjuanrivvera/jwpubkit/internal/content"
)

// glossaryClass is the document class a Bible gives its glossary of terms.
// Verified identical in Spanish and Japanese; the title is not.
const glossaryClass = 116

// GlossaryEntry is one defined term.
type GlossaryEntry struct {
	Term  string `json:"term"`
	Text  string `json:"definition"`
	DocID int    `json:"docid"`
	PID   int    `json:"pid"`
	URL   string `json:"url"`
}

// indexGlossary records the entries of a glossary document. The entries are
// marked in the markup by untranslated class names, which is what lets them be
// split into term and definition in any language.
func (ix *indexer) indexGlossary(docid int, html string) error {
	doc, err := content.Parse(html)
	if err != nil {
		return nil // a glossary we cannot parse is not worth failing a sync over
	}
	ins, err := ix.tx.Prepare(`INSERT OR REPLACE INTO glossary(pub_id, docid, pid, key, term, text) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	// Spellings that fold to the same key are the same way in, and the row is
	// keyed by that fold — so the first spelling has to win. A publication
	// writes the principal form first ("Bramblewort, Bramble-wort"), and
	// letting the last one overwrite it would display the variant as if it
	// were the word.
	seen := map[string]bool{}
	for _, b := range doc.Blocks {
		if b.Kind != content.KindDefinition || b.Term == "" {
			continue
		}
		// One entry can define several forms of a word; each is a way in.
		for _, term := range strings.Split(b.Term, ",") {
			key := FoldTerm(term)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			if _, err := ins.Exec(ix.pubID, docid, b.PID, key, strings.TrimSpace(term), b.Text()); err != nil {
				return err
			}
		}
	}
	return nil
}

// FoldTerm normalises a term for lookup: lowercase, no diacritics, no
// punctuation. Publications write a term with a trailing stop, in italics, or
// with a variant spelling, and none of that should decide whether it is found.
func FoldTerm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case 'á', 'à', 'ä', 'â':
			r = 'a'
		case 'é', 'è', 'ë', 'ê':
			r = 'e'
		case 'í', 'ì', 'ï', 'î':
			r = 'i'
		case 'ó', 'ò', 'ö', 'ô':
			r = 'o'
		case 'ú', 'ù', 'ü', 'û':
			r = 'u'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Define looks a term up in whatever glossary the library holds. It returns
// false rather than a guess: a note that points at a term the library cannot
// define should say so, not show something adjacent.
func (s *Store) Define(term string) (GlossaryEntry, bool) {
	key := FoldTerm(term)
	if key == "" {
		return GlossaryEntry{}, false
	}
	var e GlossaryEntry
	err := s.queryRow(`SELECT term, text, docid, pid FROM glossary WHERE key=? LIMIT 1`, key).
		Scan(&e.Term, &e.Text, &e.DocID, &e.PID)
	if err != nil {
		return GlossaryEntry{}, false
	}
	e.URL = content.DocURL(e.DocID, e.PID)
	return e, true
}

// GlossarySize is how many terms the library can define, which is what makes an
// unanswered lookup explainable.
func (s *Store) GlossarySize() int {
	var n int
	_ = s.DB.QueryRow(`SELECT count(DISTINCT key) FROM glossary`).Scan(&n)
	return n
}

// Terms is every term the library can define, for listing or completion.
func (s *Store) Terms(prefix string, limit int) ([]GlossaryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT term, text, docid, pid FROM glossary`
	args := []any{}
	if key := FoldTerm(prefix); key != "" {
		q += ` WHERE key LIKE ?`
		args = append(args, key+"%")
	}
	q += ` ORDER BY key LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlossaryEntry
	for rows.Next() {
		var e GlossaryEntry
		if err := rows.Scan(&e.Term, &e.Text, &e.DocID, &e.PID); err != nil {
			return nil, err
		}
		e.URL = content.DocURL(e.DocID, e.PID)
		out = append(out, e)
	}
	return out, rows.Err()
}
