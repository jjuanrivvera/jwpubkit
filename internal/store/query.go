package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jjuanrivvera/jwpubkit/internal/content"
)

// Pub is a publication of the library.
type Pub struct {
	ID         int64  `json:"-"`
	Key        string `json:"key"`
	Symbol     string `json:"symbol"`
	Issue      string `json:"number,omitempty"`
	MepsSymbol string `json:"meps_symbol"`
	Title      string `json:"title"`
	ShortTitle string `json:"short_title"`
	PubType    string `json:"kind"`
	File       string `json:"file"`
	MD5        string `json:"md5"`
	Size       int64  `json:"bytes"`
	SyncedAt   string `json:"synced_at"`
	Docs       int    `json:"documents"`
}

const pubCols = `p.id, p.key, p.symbol, p.issue, COALESCE(p.meps_symbol,''), COALESCE(p.title,''), COALESCE(p.short_title,''),
	COALESCE(p.pub_type,''), COALESCE(p.file,''), COALESCE(p.md5,''), COALESCE(p.size,0), COALESCE(p.synced_at,'')`

func scanPub(sc interface{ Scan(...any) error }, p *Pub) error {
	return sc.Scan(&p.ID, &p.Key, &p.Symbol, &p.Issue, &p.MepsSymbol, &p.Title, &p.ShortTitle, &p.PubType, &p.File, &p.MD5, &p.Size, &p.SyncedAt)
}

// Pubs lists the library.
func (s *Store) Pubs() ([]Pub, error) {
	rows, err := s.DB.Query(`SELECT ` + pubCols + `, (SELECT count(*) FROM doc d WHERE d.pub_id = p.id) FROM pub p ORDER BY p.symbol, p.issue`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pub
	for rows.Next() {
		var p Pub
		if err := rows.Scan(&p.ID, &p.Key, &p.Symbol, &p.Issue, &p.MepsSymbol, &p.Title, &p.ShortTitle, &p.PubType, &p.File, &p.MD5, &p.Size, &p.SyncedAt, &p.Docs); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PubByKey finds a publication by library key.
func (s *Store) PubByKey(key string) (*Pub, error) {
	var p Pub
	err := scanPub(s.DB.QueryRow(`SELECT `+pubCols+` FROM pub p WHERE p.key=?`, key), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

// HasSymbol reports whether any publication with that API symbol is synced.
func (s *Store) HasSymbol(symbol string) bool {
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM pub WHERE symbol=?`, symbol).Scan(&n)
	return n > 0
}

// Doc is a stored document.
type Doc struct {
	DocID        int
	Class        int
	Title        string
	TocTitle     string
	ContextTitle string
	FeatureTitle string
	FirstPage    int
	LastPage     int
	HTML         string
	Pub          Pub
}

// ChapterNumber is the number a document carries inside its publication: the
// song number in a songbook, the chapter number in a book. It is a number, so
// it reads the same in every language — unlike the heading that announces it.
// It returns false when the document is not in the library.
func (s *Store) ChapterNumber(docid int) (int, bool) {
	var n sql.NullInt64
	if err := s.DB.QueryRow(`SELECT chapter FROM doc WHERE docid=?`, docid).Scan(&n); err != nil {
		return 0, false
	}
	return int(n.Int64), n.Valid
}

// ErrNoDoc means the document is not in the library.
var ErrNoDoc = errors.New("the document is not in the library")

// Doc loads a document with its publication.
func (s *Store) Doc(docid int) (*Doc, error) {
	var d Doc
	var class, first, last sql.NullInt64
	var p Pub
	row := s.DB.QueryRow(`SELECT d.docid, d.class, COALESCE(d.title,''), COALESCE(d.toc_title,''), COALESCE(d.context_title,''),
		COALESCE(d.feature_title,''), d.first_page, d.last_page, d.html, `+pubCols+`
		FROM doc d JOIN pub p ON p.id = d.pub_id WHERE d.docid=?`, docid)
	err := row.Scan(&d.DocID, &class, &d.Title, &d.TocTitle, &d.ContextTitle, &d.FeatureTitle, &first, &last, &d.HTML,
		&p.ID, &p.Key, &p.Symbol, &p.Issue, &p.MepsSymbol, &p.Title, &p.ShortTitle, &p.PubType, &p.File, &p.MD5, &p.Size, &p.SyncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%d: %w", docid, ErrNoDoc)
	}
	if err != nil {
		return nil, err
	}
	d.Class, d.FirstPage, d.LastPage = int(class.Int64), int(first.Int64), int(last.Int64)
	d.Pub = p
	return &d, nil
}

// DocSummary is what listings show about a document.
type DocSummary struct {
	DocID   int    `json:"docid"`
	Pub     string `json:"publication"`
	Title   string `json:"title"`
	Context string `json:"context,omitempty"`
}

// Summaries loads titles for many documents at once.
func (s *Store) Summaries(ids []int) (map[int]DocSummary, error) {
	out := map[int]DocSummary{}
	for len(ids) > 0 {
		n := min(len(ids), 500)
		chunk := ids[:n]
		ids = ids[n:]
		ph := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows, err := s.DB.Query(`SELECT d.docid, COALESCE(p.meps_symbol, p.symbol), COALESCE(d.title,''), COALESCE(d.context_title,'')
			FROM doc d JOIN pub p ON p.id = d.pub_id WHERE d.docid IN (`+ph+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var d DocSummary
			if err := rows.Scan(&d.DocID, &d.Pub, &d.Title, &d.Context); err != nil {
				rows.Close()
				return nil, err
			}
			out[d.DocID] = d
		}
		rows.Close()
	}
	return out, nil
}

// SearchHit is one document found by the full-text search.
type SearchHit struct {
	DocID   int     `json:"docid"`
	Pub     string  `json:"publication"`
	PubKey  string  `json:"key"`
	Title   string  `json:"title"`
	PID     int     `json:"pid"`
	Num     int     `json:"paragraph,omitempty"`
	Snippet string  `json:"snippet"`
	Matches int     `json:"matches"`
	Rank    float64 `json:"rank"`
	URL     string  `json:"url"`
	// Issue is the publication's number, kept so a hit can be cited the way
	// publications cite themselves.
	Issue string `json:"number,omitempty"`
	// Cite is that citation, e.g. "w24.05 par. 3".
	Cite string `json:"cite"`
}

// FTSQuery turns user words into an FTS5 query: every word must appear,
// quoted phrases stay phrases, and a trailing * keeps its prefix meaning.
// Accents do not matter (the index strips them).
func FTSQuery(q string) string {
	var terms []string
	for _, part := range splitQuoted(q) {
		if part.phrase {
			if words := ftsWords(part.text); len(words) > 0 {
				terms = append(terms, `"`+strings.Join(words, " ")+`"`)
			}
			continue
		}
		for _, w := range strings.FieldsFunc(part.text, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '*'
		}) {
			prefix := strings.HasSuffix(w, "*")
			w = strings.Trim(w, "*")
			if w == "" {
				continue
			}
			t := `"` + w + `"`
			if prefix {
				t += "*"
			}
			terms = append(terms, t)
		}
	}
	return strings.Join(terms, " ")
}

func ftsWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

type queryPart struct {
	text   string
	phrase bool
}

// splitQuoted separates "quoted phrases" from loose words; an unmatched
// quote is ignored.
func splitQuoted(q string) []queryPart {
	var out []queryPart
	for {
		i := strings.IndexByte(q, '"')
		if i < 0 {
			break
		}
		j := strings.IndexByte(q[i+1:], '"')
		if j < 0 {
			q = q[:i] + " " + q[i+1:]
			break
		}
		out = append(out, queryPart{text: q[:i]}, queryPart{text: q[i+1 : i+1+j], phrase: true})
		q = q[i+2+j:]
	}
	return append(out, queryPart{text: q})
}

// Search runs a full-text query over paragraphs and returns the best
// paragraph of each matching document, best documents first. pubs filters by
// symbol ("w", "it"), MEPS symbol ("w13") or library key.
func (s *Store) Search(query string, pubs []string, limit int) ([]SearchHit, error) {
	fq := FTSQuery(query)
	if fq == "" {
		return nil, errors.New("the query is empty")
	}
	where := ""
	args := []any{fq}
	if len(pubs) > 0 {
		var ors []string
		for _, p := range pubs {
			ors = append(ors, "pub.symbol = ? OR pub.meps_symbol = ? OR pub.undated_symbol = ? OR pub.key = ?")
			args = append(args, p, p, p, p)
		}
		where = " AND (" + strings.Join(ors, " OR ") + ")"
	}
	args = append(args, limit)
	q := `WITH hits AS (
		SELECT par.docid, par.pid, par.num, bm25(par_fts) AS rank,
			snippet(par_fts, 0, '«', '»', '…', 14) AS snip
		FROM par_fts JOIN par ON par.id = par_fts.rowid
		WHERE par_fts MATCH ?
	), ranked AS (
		SELECT hits.*, count(*) OVER (PARTITION BY hits.docid) AS n,
			row_number() OVER (PARTITION BY hits.docid ORDER BY hits.rank) AS rn
		FROM hits
	)
	SELECT ranked.docid, COALESCE(pub.meps_symbol, pub.symbol), pub.key, COALESCE(doc.title,''), ranked.pid, COALESCE(ranked.num,0),
		ranked.snip, ranked.n, ranked.rank, COALESCE(pub.issue,'')
	FROM ranked JOIN doc ON doc.docid = ranked.docid JOIN pub ON pub.id = doc.pub_id
	WHERE ranked.rn = 1` + where + `
	ORDER BY ranked.rank - ln(ranked.n) LIMIT ?`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query %q: %w", fq, err)
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.DocID, &h.Pub, &h.PubKey, &h.Title, &h.PID, &h.Num, &h.Snippet, &h.Matches, &h.Rank, &h.Issue); err != nil {
			return nil, err
		}
		h.URL = content.DocURL(h.DocID, h.PID)
		h.Cite = content.Cite("", h.Pub, h.Issue, 0, h.Num, h.DocID, h.PID).Text
		out = append(out, h)
	}
	return out, rows.Err()
}

// VerseHit is a Bible verse found by the full-text search.
type VerseHit struct {
	ID      int    `json:"id"`
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
	Verse   int    `json:"verse"`
	Snippet string `json:"snippet"`
}

// SearchVerses searches the Bible text.
func (s *Store) SearchVerses(query string, limit int) ([]VerseHit, error) {
	fq := FTSQuery(query)
	if fq == "" {
		return nil, errors.New("the query is empty")
	}
	rows, err := s.DB.Query(`SELECT verse.id, verse.book, verse.chapter, verse.verse, snippet(verse_fts, 0, '«', '»', '…', 24)
		FROM verse_fts JOIN verse ON verse.id = verse_fts.rowid WHERE verse_fts MATCH ? ORDER BY verse.id LIMIT ?`, fq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VerseHit
	for rows.Next() {
		var h VerseHit
		if err := rows.Scan(&h.ID, &h.Book, &h.Chapter, &h.Verse, &h.Snippet); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Verse is one verse of the study Bible with its apparatus.
type Verse struct {
	ID        int         `json:"id"`
	Book      int         `json:"book"`
	Chapter   int         `json:"chapter"`
	Verse     int         `json:"verse"`
	Text      string      `json:"text"`
	Footnotes []Footnote  `json:"footnotes,omitempty"`
	XRefs     []XRef      `json:"marginal_references,omitempty"`
	Notes     []StudyNote `json:"study_notes,omitempty"`
}

// Footnote of the Bible text.
type Footnote struct {
	Marker string `json:"letter"`
	Anchor string `json:"anchor"`
	Text   string `json:"text"`
}

// XRef is one marginal reference letter and its targets (verse ids).
type XRef struct {
	Marker  string   `json:"letter"`
	Anchor  string   `json:"anchor"`
	Targets [][2]int `json:"-"`
	Refs    []string `json:"references"`
}

// StudyNote is a study note (nota de estudio) attached to a verse.
type StudyNote struct {
	Label string `json:"label"`
	Text  string `json:"text"`
	DocID int    `json:"docid,omitempty"`
	// Defines are the dictionary entries this note sends the reader to. A note
	// that says "see Glossary, X" expresses it as a link carrying a document id,
	// not as prose, so the pointer survives translation and can be followed.
	Defines []Definition `json:"defines,omitempty"`
}

// Definition is a term a study note points at, with where to read it.
type Definition struct {
	Term  string `json:"term"`
	DocID int    `json:"docid"`
	URL   string `json:"url"`
}

// HasBible reports whether a Bible with verse text is synced.
func (s *Store) HasBible() bool {
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM verse`).Scan(&n)
	return n > 0
}

// BibleTitle is the name the synced Bible gives itself, which is the only
// language-correct way to label the text: the tool has no business hardcoding a
// translation's name in one language when the library may hold any of them.
func (s *Store) BibleTitle() string {
	var title string
	s.DB.QueryRow(`SELECT COALESCE(p.title, '') FROM pub p
		JOIN verse v ON v.pub_id = p.id GROUP BY p.id ORDER BY count(*) DESC LIMIT 1`).Scan(&title)
	return title
}

// Verses loads verses first..last (BibleVerseId) with footnotes, marginal
// references and study notes.
func (s *Store) Verses(first, last int) ([]Verse, error) {
	rows, err := s.DB.Query(`SELECT id, book, chapter, verse, text FROM verse WHERE id BETWEEN ? AND ? ORDER BY id`, first, last)
	if err != nil {
		return nil, err
	}
	var out []Verse
	idx := map[int]int{}
	for rows.Next() {
		var v Verse
		if err := rows.Scan(&v.ID, &v.Book, &v.Chapter, &v.Verse, &v.Text); err != nil {
			rows.Close()
			return nil, err
		}
		idx[v.ID] = len(out)
		out = append(out, v)
	}
	rows.Close()
	if len(out) == 0 {
		return nil, nil
	}

	frows, err := s.DB.Query(`SELECT verse_id, COALESCE(marker,''), COALESCE(anchor,''), text FROM verse_fn WHERE verse_id BETWEEN ? AND ? ORDER BY verse_id, fnid`, first, last)
	if err != nil {
		return nil, err
	}
	for frows.Next() {
		var id int
		var f Footnote
		if err := frows.Scan(&id, &f.Marker, &f.Anchor, &f.Text); err != nil {
			frows.Close()
			return nil, err
		}
		if i, ok := idx[id]; ok {
			out[i].Footnotes = append(out[i].Footnotes, f)
		}
	}
	frows.Close()

	xrows, err := s.DB.Query(`SELECT verse_id, mid, COALESCE(marker,''), COALESCE(anchor,''), first, last FROM verse_xref
		WHERE verse_id BETWEEN ? AND ? ORDER BY verse_id, mid, seq`, first, last)
	if err != nil {
		return nil, err
	}
	lastMid := map[int]int{}
	for xrows.Next() {
		var id, mid, f, l int
		var marker, anchor string
		if err := xrows.Scan(&id, &mid, &marker, &anchor, &f, &l); err != nil {
			xrows.Close()
			return nil, err
		}
		i, ok := idx[id]
		if !ok {
			continue
		}
		v := &out[i]
		if prev, seen := lastMid[id]; !seen || prev != mid || len(v.XRefs) == 0 {
			v.XRefs = append(v.XRefs, XRef{Marker: marker, Anchor: anchor})
			lastMid[id] = mid
		}
		x := &v.XRefs[len(v.XRefs)-1]
		x.Targets = append(x.Targets, [2]int{f, l})
	}
	xrows.Close()

	nrows, err := s.DB.Query(`SELECT verse_id, COALESCE(label,''), text, COALESCE(docid,0), COALESCE(html,'') FROM verse_note WHERE verse_id BETWEEN ? AND ? ORDER BY verse_id, seq`, first, last)
	if err != nil {
		return nil, err
	}
	defer nrows.Close()
	for nrows.Next() {
		var id int
		var n StudyNote
		var html string
		if err := nrows.Scan(&id, &n.Label, &n.Text, &n.DocID, &html); err != nil {
			return nil, err
		}
		n.Defines = definitionsIn(html)
		if i, ok := idx[id]; ok {
			out[i].Notes = append(out[i].Notes, n)
		}
	}
	return out, nrows.Err()
}

// Citation is a document that cites a verse.
type Citation struct {
	DocID int    `json:"docid"`
	Pub   string `json:"publication"`
	Title string `json:"title"`
	PIDs  []int  `json:"pids"`
	URL   string `json:"url"`
	Year  int    `json:"year,omitempty"`
}

// CitedBy lists documents whose BibleCitation rows cover any verse in
// first..last, newest publications first.
func (s *Store) CitedBy(first, last, limit int) ([]Citation, int, error) {
	rows, err := s.DB.Query(`SELECT c.docid, COALESCE(p.meps_symbol, p.symbol), COALESCE(d.title,''), COALESCE(c.pid,0), COALESCE(p.year,0)
		FROM cite c JOIN doc d ON d.docid = c.docid JOIN pub p ON p.id = c.pub_id
		WHERE c.first <= ? AND c.last >= ?
		ORDER BY p.year DESC, c.docid, c.pid`, last, first)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Citation
	pos := map[int]int{}
	for rows.Next() {
		var c Citation
		var pid, year int
		if err := rows.Scan(&c.DocID, &c.Pub, &c.Title, &pid, &year); err != nil {
			return nil, 0, err
		}
		i, ok := pos[c.DocID]
		if !ok {
			c.URL = content.DocURL(c.DocID, 0)
			c.Year = year
			pos[c.DocID] = len(out)
			out = append(out, c)
			i = len(out) - 1
		}
		if pid > 0 && !containsInt(out[i].PIDs, pid) {
			out[i].PIDs = append(out[i].PIDs, pid)
		}
	}
	total := len(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total, rows.Err()
}

// ParagraphText concatenates the text of the paragraph(s) stored for docid
// at pid (normally one row; a handful of layouts split one pid into more
// than one par row).
func (s *Store) ParagraphText(docid, pid int) (string, error) {
	rows, err := s.DB.Query(`SELECT text FROM par WHERE docid=? AND pid=? ORDER BY id`, docid, pid)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return "", err
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " "), rows.Err()
}

// DocByTitle finds a document of a publication (by API symbol, MEPS symbol
// or undated symbol, e.g. "it") whose title matches exactly, ignoring case
// and surrounding whitespace. Used to cross-reference a term against
// Perspicacia.
func (s *Store) DocByTitle(pubSymbol, title string) (*DocSummary, error) {
	var d DocSummary
	err := s.DB.QueryRow(`SELECT d.docid, COALESCE(p.meps_symbol, p.symbol), COALESCE(d.title,'')
		FROM doc d JOIN pub p ON p.id = d.pub_id
		WHERE (p.symbol=?1 OR p.meps_symbol=?1 OR p.undated_symbol=?1) AND lower(trim(d.title)) = lower(trim(?2))
		LIMIT 1`, pubSymbol, title).Scan(&d.DocID, &d.Pub, &d.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Media is a DocumentMultimedia row.
type Media struct {
	DocID     int
	BeginPID  int
	EndPID    int
	DataType  int
	Mime      string
	Width     int
	Height    int
	Label     string
	Caption   string
	Category  int
	File      string
	KeySymbol string
	Track     int
	MepsDocID int
	IssueTag  int
	PubFile   string // .jwpub that holds the file
}

// MediaOf lists the multimedia of a document.
func (s *Store) MediaOf(docid int) ([]Media, error) {
	rows, err := s.DB.Query(`SELECT m.docid, COALESCE(m.begin_pid,0), COALESCE(m.end_pid,0), COALESCE(m.data_type,0), COALESCE(m.mime,''),
		COALESCE(m.width,0), COALESCE(m.height,0), COALESCE(m.label,''), COALESCE(m.caption,''), COALESCE(m.category,0),
		COALESCE(m.file,''), COALESCE(m.key_symbol,''), COALESCE(m.track,0), COALESCE(m.meps_docid,0), COALESCE(m.issue_tag,0), COALESCE(p.file,'')
		FROM media m JOIN pub p ON p.id = m.pub_id WHERE m.docid=? ORDER BY m.begin_pid, m.mm_id`, docid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.DocID, &m.BeginPID, &m.EndPID, &m.DataType, &m.Mime, &m.Width, &m.Height, &m.Label, &m.Caption,
			&m.Category, &m.File, &m.KeySymbol, &m.Track, &m.MepsDocID, &m.IssueTag, &m.PubFile); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Extract is referenced text a publication ships (the paragraphs a Meeting
// Workbook cites, a whole chapter of the congregation study book...).
type Extract struct {
	DocID      int    // document holding the reference
	BeginPID   int    // where in that document
	Link       string // "p/S:2013043/22-22"
	Caption    string // where the citation sits, as the publication writes it
	Title      string // the citing document's title
	RefDocID   int
	RefClass   int
	RefBegin   int
	RefEnd     int
	RefSymbol  string // "w13"
	RefUndated string // "w"
	RefIssue   int    // 20130115
	RefTitle   string // "La Atalaya 2013"
	HTML       string
	PubFile    string
}

const extractCols = `e.docid, COALESCE(e.begin_pid,0), COALESCE(e.link,''), COALESCE(e.caption,''), COALESCE(e.title,''),
	COALESCE(e.ref_docid,0), COALESCE(e.ref_class,0), COALESCE(e.ref_begin,0), COALESCE(e.ref_end,0), COALESCE(e.ref_symbol,''),
	COALESCE(e.ref_undated,''), COALESCE(e.ref_issue_tag,0), COALESCE(e.ref_title,''), COALESCE(e.html,''), COALESCE(p.file,'')`

func scanExtracts(rows *sql.Rows) ([]Extract, error) {
	defer rows.Close()
	var out []Extract
	for rows.Next() {
		var e Extract
		if err := rows.Scan(&e.DocID, &e.BeginPID, &e.Link, &e.Caption, &e.Title, &e.RefDocID, &e.RefClass, &e.RefBegin, &e.RefEnd,
			&e.RefSymbol, &e.RefUndated, &e.RefIssue, &e.RefTitle, &e.HTML, &e.PubFile); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExtractsOf lists the extracts a document carries, in reading order.
func (s *Store) ExtractsOf(docid int) ([]Extract, error) {
	rows, err := s.DB.Query(`SELECT `+extractCols+` FROM extract e JOIN pub p ON p.id = e.pub_id WHERE e.docid=? ORDER BY e.sort`, docid)
	if err != nil {
		return nil, err
	}
	return scanExtracts(rows)
}

// ExtractsFor lists extracts of other publications that quote refDocID.
func (s *Store) ExtractsFor(refDocID int) ([]Extract, error) {
	rows, err := s.DB.Query(`SELECT `+extractCols+` FROM extract e JOIN pub p ON p.id = e.pub_id WHERE e.ref_docid=?
		ORDER BY length(e.html) DESC`, refDocID)
	if err != nil {
		return nil, err
	}
	return scanExtracts(rows)
}

// DatedDocs returns the documents of publications with API symbol whose
// DatedText covers date (YYYYMMDD), with the dated link.
func (s *Store) DatedDocs(symbol string, date int) ([]DatedDoc, error) {
	rows, err := s.DB.Query(`SELECT dated.docid, dated.first, dated.last, COALESCE(dated.link,''), COALESCE(dated.caption,''), pub.key
		FROM dated JOIN pub ON pub.id = dated.pub_id
		WHERE pub.symbol = ? AND dated.first <= ? AND dated.last >= ? ORDER BY pub.issue DESC`, symbol, date, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DatedDoc
	for rows.Next() {
		var d DatedDoc
		if err := rows.Scan(&d.DocID, &d.First, &d.Last, &d.Link, &d.Caption, &d.PubKey); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DatedDoc is a DatedText row.
type DatedDoc struct {
	DocID   int
	First   int
	Last    int
	Link    string
	Caption string
	PubKey  string
}

// StudyArticles lists the Watchtower study articles (class 40) of a
// publication in their order; M³ maps weeks to them by position.
func (s *Store) StudyArticles(pubKey string) ([]int, error) {
	rows, err := s.DB.Query(`SELECT d.docid FROM doc d JOIN pub p ON p.id = d.pub_id WHERE p.key=? AND d.class=40 ORDER BY d.local_id`, pubKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DatedRank is the position of a DatedText date inside its publication.
func (s *Store) DatedRank(pubKey string, first int) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM dated JOIN pub ON pub.id = dated.pub_id WHERE pub.key=? AND dated.first < ?`, pubKey, first).Scan(&n)
	return n, err
}

// CachedVideo is a mediator answer kept in the library.
type CachedVideo struct {
	Key       string
	Title     string
	Duration  float64
	Subtitles string
	JSON      string
}

// Video returns a cached mediator item.
func (s *Store) Video(key, lang string) (*CachedVideo, error) {
	var v CachedVideo
	err := s.DB.QueryRow(`SELECT key, COALESCE(title,''), COALESCE(duration,0), COALESCE(subtitles,''), COALESCE(json,'') FROM video WHERE key=? AND lang=?`, key, lang).
		Scan(&v.Key, &v.Title, &v.Duration, &v.Subtitles, &v.JSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}

// PutVideo caches a mediator item.
func (s *Store) PutVideo(key, lang, title string, duration float64, subtitles string, raw any) error {
	b, _ := json.Marshal(raw)
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO video(key, lang, title, duration, subtitles, json, fetched_at) VALUES(?,?,?,?,?,?,?)`,
		key, lang, title, duration, subtitles, string(b), time.Now().Format(time.RFC3339))
	return err
}

// dictLinkRe finds the links a study note uses to send the reader to a
// dictionary entry: class="xt" with a publication link carrying a document id
// and no paragraph. The class name and the jwpub scheme are untranslated, which
// is why they can be matched while the words around them cannot.
var dictLinkRe = regexp.MustCompile(`<a[^>]*class="xt"[^>]*href="jwpub://p/[A-Za-z]+:(\d+)/?"[^>]*>(.*?)</a>`)

var tagRe = regexp.MustCompile(`<[^>]+>`)

// definitionsIn pulls the dictionary entries a note points at. The entries
// themselves are published online rather than inside any JWPUB, so what can be
// given is the identifier and the address — which are correct in every
// language — rather than a definition the library does not hold.
func definitionsIn(html string) []Definition {
	if html == "" {
		return nil
	}
	var out []Definition
	seen := map[int]bool{}
	for _, m := range dictLinkRe.FindAllStringSubmatch(html, -1) {
		docid, err := strconv.Atoi(m[1])
		if err != nil || seen[docid] {
			continue
		}
		seen[docid] = true
		term := strings.TrimSpace(tagRe.ReplaceAllString(m[2], ""))
		out = append(out, Definition{Term: term, DocID: docid, URL: content.DocURL(docid, 0)})
	}
	return out
}
