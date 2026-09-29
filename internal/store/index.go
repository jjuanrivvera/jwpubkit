package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/jwpub"
)

// PubInfo describes where a JWPUB came from.
type PubInfo struct {
	Symbol   string // API symbol: mwb, w, nwtsty
	Issue    string // "202609", "" for books
	Lang     string
	File     string // path of the cached .jwpub
	MD5      string
	Size     int64
	Modified string
}

// Key is the library key of a publication: "mwb_S_202609", "nwtsty_S".
func (p PubInfo) Key() string { return PubKey(p.Symbol, p.Lang, p.Issue) }

// PubKey builds the library key.
func PubKey(symbol, lang, issue string) string {
	k := symbol + "_" + lang
	if issue != "" {
		k += "_" + issue
	}
	return k
}

// IndexStats counts what an index run stored.
type IndexStats struct {
	Docs, Pars, Verses, Notes, Footnotes, XRefs, Cites, Media, Extracts, Dated, Questions int
	Elapsed                                                                               time.Duration
}

func (st IndexStats) String() string {
	s := fmt.Sprintf("%d documents, %d paragraphs", st.Docs, st.Pars)
	if st.Verses > 0 {
		s += fmt.Sprintf(", %d verses, %d study notes, %d footnotes, %d marginal references", st.Verses, st.Notes, st.Footnotes, st.XRefs)
	}
	s += fmt.Sprintf(", %d bible citations, %d media, %d extracts", st.Cites, st.Media, st.Extracts)
	if st.Dated > 0 {
		s += fmt.Sprintf(", %d semanas", st.Dated)
	}
	return s
}

// indexer carries the open JWPUB and the transaction.
type indexer struct {
	jf    *jwpub.File
	tx    *sql.Tx
	lang  string
	pubID int64
	stats IndexStats
	docs  map[int]int // local DocumentId -> MepsDocumentId
	mids  map[markKey]content.Mark
}

// markKey scopes footnote and marginal ids: they restart in every book.
type markKey struct{ book, id int }

func bookOf(verseID int64) int {
	b, _, _, _ := bible.Locate(int(verseID))
	return b
}

// Index decrypts and stores a JWPUB, replacing what the library had for it.
func (s *Store) Index(info PubInfo) (*IndexStats, error) {
	start := time.Now()
	jf, err := jwpub.OpenIn(info.File, filepath.Join(s.Dir, "tmp"))
	if err != nil {
		return nil, err
	}
	defer jf.Close()

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ix := &indexer{jf: jf, tx: tx, lang: info.Lang, docs: map[int]int{}}
	if err := ix.upsertPub(info); err != nil {
		return nil, err
	}
	steps := []struct {
		name string
		fn   func() error
	}{
		{"documentos", ix.documents},
		{"medios", ix.media},
		{"extractos", ix.extracts},
		{"verses", ix.verses}, // before marginalRefs: it finds the letters
		{"referencias marginales", ix.marginalRefs},
		{"fechas", ix.dated},
		{"preguntas", ix.questions},
		{"notas de estudio", ix.studyNotes},
	}
	for _, step := range steps {
		if err := step.fn(); err != nil {
			return nil, fmt.Errorf("indexando %s de %s: %w", step.name, info.Key(), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ix.stats.Elapsed = time.Since(start)
	return &ix.stats, nil
}

func (ix *indexer) upsertPub(info PubInfo) error {
	jf := ix.jf
	var p struct {
		title, short, symbol, undated, ptype, cat sql.NullString
		year, issueTag, lang                      sql.NullInt64
		first, last                               sql.NullInt64
	}
	q := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s FROM Publication LIMIT 1`,
		jf.Col("Publication", "Title"), jf.Col("Publication", "ShortTitle"), jf.Col("Publication", "Symbol"),
		jf.Col("Publication", "UndatedSymbol"), jf.Col("Publication", "PublicationType"),
		jf.Col("Publication", "PublicationCategorySymbol"), jf.Col("Publication", "Year"),
		jf.Col("Publication", "IssueTagNumber"), jf.Col("Publication", "MepsLanguageIndex"),
		jf.Col("Publication", "FirstDatedTextDateOffset"), jf.Col("Publication", "LastDatedTextDateOffset"))
	if err := jf.DB.QueryRow(q).Scan(&p.title, &p.short, &p.symbol, &p.undated, &p.ptype, &p.cat,
		&p.year, &p.issueTag, &p.lang, &p.first, &p.last); err != nil {
		return fmt.Errorf("Publication: %w", err)
	}
	key := info.Key()
	var id int64
	err := ix.tx.QueryRow(`SELECT id FROM pub WHERE key=?`, key).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		res, err := ix.tx.Exec(`INSERT INTO pub(key, symbol, issue, lang) VALUES(?,?,?,?)`, key, info.Symbol, info.Issue, info.Lang)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
	case err != nil:
		return err
	default:
		if err := ix.purge(id); err != nil {
			return err
		}
	}
	ix.pubID = id
	_, err = ix.tx.Exec(`UPDATE pub SET meps_symbol=?, undated_symbol=?, meps_lang=?, year=?, issue_tag=?,
		title=?, short_title=?, pub_type=?, category=?, file=?, md5=?, size=?, modified=?, synced_at=?,
		first_date=?, last_date=? WHERE id=?`,
		p.symbol.String, p.undated.String, p.lang.Int64, p.year.Int64, p.issueTag.Int64,
		p.title.String, p.short.String, p.ptype.String, p.cat.String,
		info.File, info.MD5, info.Size, info.Modified, time.Now().Format(time.RFC3339),
		p.first.Int64, p.last.Int64, id)
	return err
}

// purge removes every row a previous index of this publication left.
func (ix *indexer) purge(pubID int64) error {
	stmts := []string{
		`DELETE FROM par WHERE docid IN (SELECT docid FROM doc WHERE pub_id=?1)`,
		`DELETE FROM question WHERE docid IN (SELECT docid FROM doc WHERE pub_id=?1)`,
		`DELETE FROM doc WHERE pub_id=?1`,
		`DELETE FROM verse WHERE pub_id=?1`,
		`DELETE FROM verse_note WHERE pub_id=?1`,
		`DELETE FROM verse_fn WHERE pub_id=?1`,
		`DELETE FROM verse_xref WHERE pub_id=?1`,
		`DELETE FROM cite WHERE pub_id=?1`,
		`DELETE FROM media WHERE pub_id=?1`,
		`DELETE FROM extract WHERE pub_id=?1`,
		`DELETE FROM dated WHERE pub_id=?1`,
	}
	for _, q := range stmts {
		if _, err := ix.tx.Exec(q, pubID); err != nil {
			return err
		}
	}
	return nil
}

func (ix *indexer) documents() error {
	jf := ix.jf
	d := "Document"
	q := fmt.Sprintf(`SELECT DocumentId, MepsDocumentId, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, Content FROM Document ORDER BY DocumentId`,
		jf.Col(d, "Class"), jf.Col(d, "Type"), jf.Col(d, "SectionNumber"), jf.Col(d, "ChapterNumber"),
		jf.Col(d, "Title"), jf.Col(d, "TocTitle"), jf.Col(d, "ContextTitle"), jf.Col(d, "FeatureTitle"),
		jf.Col(d, "FirstPageNumber"), jf.Col(d, "LastPageNumber"))
	rows, err := jf.DB.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()

	delPars, err := ix.tx.Prepare(`DELETE FROM par WHERE docid=?`)
	if err != nil {
		return err
	}
	defer delPars.Close()
	insDoc, err := ix.tx.Prepare(`INSERT OR REPLACE INTO doc(docid, pub_id, local_id, class, type, section, chapter,
		title, toc_title, context_title, feature_title, first_page, last_page, html) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insDoc.Close()
	insPar, err := ix.tx.Prepare(`INSERT INTO par(docid, pid, num, sub, kind, text) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insPar.Close()
	insCite, err := ix.tx.Prepare(`INSERT INTO cite(docid, pid, first, last, pub_id) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insCite.Close()

	for rows.Next() {
		var localID, docid int
		var class, typ, section, chapter, first, last sql.NullInt64
		var title, toc, ctx, feature sql.NullString
		var blob []byte
		if err := rows.Scan(&localID, &docid, &class, &typ, &section, &chapter, &title, &toc, &ctx, &feature, &first, &last, &blob); err != nil {
			return err
		}
		ix.docs[localID] = docid
		var htmlText string
		if len(blob) > 0 {
			htmlText, err = jf.Decrypt(blob)
			if err != nil {
				return fmt.Errorf("document %d: %w", docid, err)
			}
		}
		if _, err := delPars.Exec(docid); err != nil {
			return err
		}
		if _, err := insDoc.Exec(docid, ix.pubID, localID, class, typ, section, chapter,
			title.String, toc.String, ctx.String, feature.String, first, last, htmlText); err != nil {
			return err
		}
		ix.stats.Docs++
		if htmlText == "" {
			continue
		}
		parsed, err := content.Parse(htmlText)
		if err != nil {
			return fmt.Errorf("document %d: %w", docid, err)
		}
		for _, b := range parsed.Blocks {
			text := b.Text()
			if text == "" {
				continue
			}
			if _, err := insPar.Exec(docid, b.PID, b.Num, b.Sub, b.Kind, text); err != nil {
				return err
			}
			ix.stats.Pars++
			// Citations come from the links (book:chapter:verse), not from
			// BibleCitation: its verse ids follow the Bible edition of the
			// publication (NWT in a 2013 Watchtower), not the current NWTR.
			for _, r := range b.BibleRefs() {
				first, ok1 := bible.VerseID(r.Book, r.StartChapter, r.StartVerse)
				last, ok2 := bible.VerseID(r.Book, r.EndChapter, r.EndVerse)
				if !ok1 || !ok2 {
					continue
				}
				if _, err := insCite.Exec(docid, b.PID, first, last, ix.pubID); err != nil {
					return err
				}
				ix.stats.Cites++
			}
		}
	}
	return rows.Err()
}

func (ix *indexer) media() error {
	jf := ix.jf
	if !jf.HasTable("Multimedia") {
		return nil
	}
	m, dm := "Multimedia", "DocumentMultimedia"
	from := "FROM DocumentMultimedia JOIN Multimedia ON Multimedia.MultimediaId = DocumentMultimedia.MultimediaId"
	docCol := "DocumentMultimedia.DocumentId"
	if !jf.HasTable(dm) {
		// Schema 6 and older: each image row names its document.
		if !jf.Columns(m)["DocumentId"] {
			return nil
		}
		from, docCol = "FROM Multimedia", "Multimedia.DocumentId"
	}
	q := fmt.Sprintf(`SELECT %s, Multimedia.MultimediaId, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
		%s`, docCol,
		jf.Col(dm, "BeginParagraphOrdinal"), jf.Col(dm, "EndParagraphOrdinal"),
		jf.Col(m, "DataType"), jf.Col(m, "MimeType"), jf.Col(m, "Width"), jf.Col(m, "Height"),
		jf.Col(m, "Label"), jf.Col(m, "Caption"), jf.Col(m, "CategoryType"), jf.Col(m, "FilePath"),
		jf.Col(m, "KeySymbol"), jf.Col(m, "Track"), jf.Col(m, "MepsDocumentId"), jf.Col(m, "IssueTagNumber"), from)
	rows, err := jf.DB.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()
	ins, err := ix.tx.Prepare(`INSERT INTO media(pub_id, docid, mm_id, begin_pid, end_pid, data_type, mime, width, height,
		label, caption, category, file, key_symbol, track, meps_docid, issue_tag) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for rows.Next() {
		var local, mmID int
		var begin, end, dtype, width, height, cat, track, mdoc, issue sql.NullInt64
		var mime, label, caption, file, key sql.NullString
		if err := rows.Scan(&local, &mmID, &begin, &end, &dtype, &mime, &width, &height, &label, &caption, &cat, &file, &key, &track, &mdoc, &issue); err != nil {
			return err
		}
		docid, ok := ix.docs[local]
		if !ok {
			continue
		}
		if _, err := ins.Exec(ix.pubID, docid, mmID, begin, end, dtype, mime.String, width, height,
			label.String, caption.String, cat, file.String, key.String, track, mdoc, issue); err != nil {
			return err
		}
		ix.stats.Media++
	}
	return rows.Err()
}

var (
	elocRe   = regexp.MustCompile(`<span class="eloc">(.*?)</span>`)
	etitleRe = regexp.MustCompile(`<span class="etitle">(.*?)</span>`)
)

// SplitCaption separates the location and title of an Extract/DatedText
// caption: `<span class="eloc">…location…</span> <span class="etitle">…title…</span>`.
func SplitCaption(c string) (loc, title string) {
	if m := elocRe.FindStringSubmatch(c); m != nil {
		loc = content.InnerText(m[1])
	}
	if m := etitleRe.FindStringSubmatch(c); m != nil {
		title = content.InnerText(m[1])
	}
	if loc == "" && title == "" {
		loc = content.InnerText(c)
	}
	return loc, title
}

func (ix *indexer) extracts() error {
	jf := ix.jf
	if !jf.HasTable("DocumentExtract") || !jf.HasTable("Extract") {
		return nil
	}
	e, de, rp := "Extract", "DocumentExtract", "RefPublication"
	join := ""
	refCols := "NULL, NULL, NULL, NULL"
	if jf.HasTable(rp) && jf.Columns(e)["RefPublicationId"] {
		join = "LEFT JOIN RefPublication ON RefPublication.RefPublicationId = Extract.RefPublicationId"
		refCols = fmt.Sprintf("%s, %s, %s, %s", jf.Col(rp, "Symbol"), jf.Col(rp, "UndatedSymbol"), jf.Col(rp, "IssueTagNumber"), jf.Col(rp, "ShortTitle"))
	}
	q := fmt.Sprintf(`SELECT DocumentExtract.DocumentId, Extract.ExtractId, %s, %s, %s, %s, %s, Extract.Content, %s, %s, %s, %s, %s
		FROM DocumentExtract JOIN Extract ON Extract.ExtractId = DocumentExtract.ExtractId %s`,
		jf.Col(de, "BeginParagraphOrdinal"), jf.Col(de, "EndParagraphOrdinal"), jf.Col(de, "SortPosition"),
		jf.Col(e, "Link"), jf.Col(e, "Caption"),
		jf.Col(e, "RefMepsDocumentId"), jf.Col(e, "RefMepsDocumentClass"), jf.Col(e, "RefBeginParagraphOrdinal"),
		jf.Col(e, "RefEndParagraphOrdinal"), refCols, join)
	rows, err := jf.DB.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()
	ins, err := ix.tx.Prepare(`INSERT INTO extract(pub_id, docid, ext_id, begin_pid, end_pid, sort, link, caption, title,
		ref_docid, ref_class, ref_begin, ref_end, ref_symbol, ref_undated, ref_issue_tag, ref_title, html)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for rows.Next() {
		var local, extID int
		var begin, end, sort, refDoc, refClass, refBegin, refEnd sql.NullInt64
		var link, caption, refSym, refUndated, refIssue, refTitle sql.NullString
		var blob []byte
		if err := rows.Scan(&local, &extID, &begin, &end, &sort, &link, &caption, &blob, &refDoc, &refClass, &refBegin, &refEnd,
			&refSym, &refUndated, &refIssue, &refTitle); err != nil {
			return err
		}
		docid, ok := ix.docs[local]
		if !ok {
			continue
		}
		var htmlText string
		if len(blob) > 0 {
			if htmlText, err = jf.Decrypt(blob); err != nil {
				return fmt.Errorf("extract %d: %w", extID, err)
			}
		}
		loc, title := SplitCaption(caption.String)
		var issueTag int64
		fmt.Sscan(refIssue.String, &issueTag)
		if _, err := ins.Exec(ix.pubID, docid, extID, begin, end, sort, link.String, loc, title,
			refDoc, refClass, refBegin, refEnd, refSym.String, refUndated.String, issueTag, refTitle.String, htmlText); err != nil {
			return err
		}
		ix.stats.Extracts++
	}
	return rows.Err()
}

// marginalRefs stores the marginal references of a Bible: BibleCitation
// rows with MarginalClassification=1 (a Bible's own ids are NWTR). Citations
// in documents come from their links instead (see documents).
func (ix *indexer) marginalRefs() error {
	jf := ix.jf
	if ix.stats.Verses == 0 || !jf.Columns("BibleCitation")["MarginalClassification"] {
		return nil
	}
	rows, err := jf.DB.Query(`SELECT BibleVerseId, BlockNumber, ElementNumber, FirstBibleVerseId, COALESCE(LastBibleVerseId, FirstBibleVerseId)
		FROM BibleCitation WHERE MarginalClassification = 1 AND BibleVerseId IS NOT NULL AND FirstBibleVerseId IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ins, err := ix.tx.Prepare(`INSERT INTO verse_xref(verse_id, mid, marker, anchor, seq, first, last, pub_id) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for rows.Next() {
		var verse, mid, seq, first, last int
		if err := rows.Scan(&verse, &mid, &seq, &first, &last); err != nil {
			return err
		}
		mk := ix.mids[markKey{bookOf(int64(verse)), mid}]
		if _, err := ins.Exec(verse, mid, mk.Letter, mk.Anchor, seq, first, last, ix.pubID); err != nil {
			return err
		}
		ix.stats.XRefs++
	}
	return rows.Err()
}

func (ix *indexer) dated() error {
	jf := ix.jf
	if !jf.HasTable("DatedText") {
		return nil
	}
	rows, err := jf.DB.Query(fmt.Sprintf(`SELECT DocumentId, FirstDateOffset, LastDateOffset, %s, %s FROM DatedText`,
		jf.Col("DatedText", "Link"), jf.Col("DatedText", "Caption")))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var local int
		var first, last sql.NullInt64
		var link, caption sql.NullString
		if err := rows.Scan(&local, &first, &last, &link, &caption); err != nil {
			return err
		}
		docid, ok := ix.docs[local]
		if !ok || !first.Valid {
			continue
		}
		loc, title := SplitCaption(caption.String)
		cap := strings.TrimSpace(loc + " · " + title)
		if _, err := ix.tx.Exec(`INSERT INTO dated(pub_id, docid, first, last, link, caption) VALUES(?,?,?,?,?,?)`,
			ix.pubID, docid, first.Int64, last.Int64, link.String, cap); err != nil {
			return err
		}
		ix.stats.Dated++
	}
	return rows.Err()
}

func (ix *indexer) questions() error {
	jf := ix.jf
	if !jf.HasTable("Question") {
		return nil
	}
	q := "Question"
	rows, err := jf.DB.Query(fmt.Sprintf(`SELECT DocumentId, %s, %s, %s, %s, Content FROM Question`,
		jf.Col(q, "QuestionIndex"), jf.Col(q, "ParagraphOrdinal"), jf.Col(q, "TargetParagraphOrdinal"), jf.Col(q, "TargetParagraphNumberLabel")))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var local int
		var idx, pid, target, label sql.NullInt64
		var blob []byte
		if err := rows.Scan(&local, &idx, &pid, &target, &label, &blob); err != nil {
			return err
		}
		docid, ok := ix.docs[local]
		if !ok || len(blob) == 0 {
			continue
		}
		h, err := jf.Decrypt(blob)
		if err != nil {
			return err
		}
		if _, err := ix.tx.Exec(`INSERT INTO question(docid, idx, pid, target_pid, target_num, text) VALUES(?,?,?,?,?,?)`,
			docid, idx, pid, target, label, content.InnerText(h)); err != nil {
			return err
		}
		ix.stats.Questions++
	}
	return rows.Err()
}

// verses stores the Bible text, its footnotes and the letters and anchor
// words of footnote calls and marginal references.
// bookNames records how this Bible names its books, which is what lets the CLI
// parse and print references in the library's language without carrying a
// hand-written table for each of the languages jw.org publishes.
func (ix *indexer) bookNames() error {
	jf := ix.jf
	if !jf.HasTable("BibleBook") {
		return nil
	}
	rows, err := jf.DB.Query(`SELECT BibleBookId, COALESCE(ChapterDisplayTitle, '') FROM BibleBook`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ins, err := ix.tx.Prepare(`INSERT OR REPLACE INTO book_name(lang, book, name) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for rows.Next() {
		var num int
		var name string
		if err := rows.Scan(&num, &name); err != nil {
			return err
		}
		if num < 1 || num > 66 || strings.TrimSpace(name) == "" {
			continue
		}
		if _, err := ins.Exec(ix.lang, num, name); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (ix *indexer) verses() error {
	jf := ix.jf
	if !jf.HasTable("BibleVerse") || !jf.HasTable("BibleChapter") {
		return nil
	}
	if err := ix.bookNames(); err != nil {
		return err
	}
	rows, err := jf.DB.Query(`SELECT BibleVerseId, Content FROM BibleVerse ORDER BY BibleVerseId`)
	if err != nil {
		return err
	}
	ins, err := ix.tx.Prepare(`INSERT OR REPLACE INTO verse(id, book, chapter, verse, text, pub_id) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		rows.Close()
		return err
	}
	defer ins.Close()
	for rows.Next() {
		var id int
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			rows.Close()
			return err
		}
		if len(blob) == 0 {
			continue
		}
		h, err := jf.Decrypt(blob)
		if err != nil {
			rows.Close()
			return fmt.Errorf("verse %d: %w", id, err)
		}
		b, c, v, text, ok := content.VerseText(h)
		if !ok {
			continue
		}
		if _, err := ins.Exec(id, b, c, v, text, ix.pubID); err != nil {
			rows.Close()
			return err
		}
		ix.stats.Verses++
	}
	rows.Close()
	if ix.stats.Verses == 0 {
		return nil
	}

	// Chapter HTML carries the letters and the words they hang from.
	fnMarks, midMarks := map[markKey]content.Mark{}, map[markKey]content.Mark{}
	crows, err := jf.DB.Query(`SELECT Content FROM BibleChapter`)
	if err != nil {
		return err
	}
	for crows.Next() {
		var blob []byte
		if err := crows.Scan(&blob); err != nil {
			crows.Close()
			return err
		}
		if len(blob) == 0 {
			continue
		}
		h, err := jf.Decrypt(blob)
		if err != nil {
			crows.Close()
			return err
		}
		fns, mids := content.ChapterMarks(h)
		for k, v := range fns {
			fnMarks[markKey{v.Book, k}] = v
		}
		for k, v := range mids {
			midMarks[markKey{v.Book, k}] = v
		}
	}
	crows.Close()

	ix.mids = midMarks

	if !jf.HasTable("Footnote") || !jf.Columns("Footnote")["BibleVerseId"] {
		return nil
	}
	frows, err := jf.DB.Query(`SELECT FootnoteIndex, BibleVerseId, Content FROM Footnote WHERE BibleVerseId IS NOT NULL`)
	if err != nil {
		return err
	}
	defer frows.Close()
	insFn, err := ix.tx.Prepare(`INSERT INTO verse_fn(verse_id, fnid, marker, anchor, text, pub_id) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insFn.Close()
	for frows.Next() {
		var fnid, verseID int
		var blob []byte
		if err := frows.Scan(&fnid, &verseID, &blob); err != nil {
			return err
		}
		h, err := jf.Decrypt(blob)
		if err != nil {
			return err
		}
		mk := fnMarks[markKey{bookOf(int64(verseID)), fnid}]
		if _, err := insFn.Exec(verseID, fnid, mk.Letter, mk.Anchor, content.InnerText(h), ix.pubID); err != nil {
			return err
		}
		ix.stats.Footnotes++
	}
	return frows.Err()
}

var noteLabelRe = regexp.MustCompile(`\s+`)

func (ix *indexer) studyNotes() error {
	jf := ix.jf
	if !jf.HasTable("VerseCommentary") || !jf.HasTable("VerseCommentaryMap") {
		return nil
	}
	vc := "VerseCommentary"
	rows, err := jf.DB.Query(fmt.Sprintf(`SELECT VerseCommentaryMap.BibleVerseId, VerseCommentary.VerseCommentaryId, %s, VerseCommentary.Content, %s
		FROM VerseCommentaryMap JOIN VerseCommentary ON VerseCommentary.VerseCommentaryId = VerseCommentaryMap.VerseCommentaryId
		ORDER BY VerseCommentaryMap.BibleVerseId, VerseCommentary.VerseCommentaryId`,
		jf.Col(vc, "Label"), jf.Col(vc, "CommentaryMepsDocumentId")))
	if err != nil {
		return err
	}
	defer rows.Close()
	ins, err := ix.tx.Prepare(`INSERT INTO verse_note(verse_id, seq, label, text, html, docid, pub_id) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for rows.Next() {
		var verseID, seq int
		var label sql.NullString
		var docid sql.NullInt64
		var blob []byte
		if err := rows.Scan(&verseID, &seq, &label, &blob, &docid); err != nil {
			return err
		}
		if len(blob) == 0 {
			continue
		}
		h, err := jf.Decrypt(blob)
		if err != nil {
			return err
		}
		lbl := noteLabelRe.ReplaceAllString(content.InnerText(label.String), " ")
		if _, err := ins.Exec(verseID, seq, lbl, content.InnerText(h), h, docid, ix.pubID); err != nil {
			return err
		}
		ix.stats.Notes++
	}
	return rows.Err()
}
