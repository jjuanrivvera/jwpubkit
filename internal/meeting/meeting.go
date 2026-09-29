// Package meeting builds the midweek meeting of a week (from the Meeting
// Workbook, mwb) and the Watchtower study article of that week (w).
package meeting

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

// Week is everything `pubkit week` reports.
type Week struct {
	Monday            string        `json:"monday"`
	Range             string        `json:"range"`
	Workbook          *DocRef       `json:"workbook"`
	WeeklyReading     *Reading      `json:"weekly_reading,omitempty"`
	StudentReading    *Assignment   `json:"student_reading,omitempty"`
	Songs             []Song        `json:"songs"`
	Sections          []Section     `json:"sections"`
	Videos            []Video       `json:"videos"`
	Images            []Image       `json:"images"`
	Watchtower        *Watchtower   `json:"watchtower,omitempty"`
	Notes             []string      `json:"notes,omitempty"`
	CongregationStudy *StudyChapter `json:"-"`
}

// DocRef identifies a document.
type DocRef struct {
	DocID    int    `json:"docid"`
	Pub      string `json:"publication"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	Location string `json:"location,omitempty"`
	URL      string `json:"url"`
}

// Reading is the weekly Bible reading.
type Reading struct {
	Text     string        `json:"text"`
	Book     string        `json:"book"`
	BookNum  int           `json:"book_number"`
	Chapters []int         `json:"chapters"`
	Ranges   []bible.Range `json:"ranges"`
	Ref      string        `json:"reference"`
}

// Assignment is the student Bible reading.
type Assignment struct {
	Ref    string       `json:"reference"`
	Range  *bible.Range `json:"range,omitempty"`
	Lesson *Reference   `json:"lesson,omitempty"`
}

// Song of the meeting.
type Song struct {
	Number int    `json:"number"`
	Title  string `json:"title,omitempty"`
	DocID  int    `json:"docid,omitempty"`
	When   string `json:"when"` // start, middle, end
	Video  string `json:"video"`
}

// Section groups the parts under one of the workbook's top-level headings.
type Section struct {
	Title string `json:"title"`
	// Marker is the untranslated icon the publication puts on this section's
	// heading, which is how a section is recognized in any language. Empty when
	// the markup carries no marker, and then the parts are still listed but
	// nothing is derived from which section they are in.
	Marker string `json:"marker,omitempty"`
	Parts  []Part `json:"parts"`
}

// Part of the meeting.
type Part struct {
	Number     int           `json:"number,omitempty"`
	Title      string        `json:"title"`
	Minutes    int           `json:"minutes,omitempty"`
	PID        int           `json:"pid"`
	Text       []string      `json:"text,omitempty"`
	Questions  []string      `json:"questions,omitempty"`
	References []Reference   `json:"references,omitempty"`
	Videos     []Video       `json:"videos,omitempty"`
	Images     []Image       `json:"images,omitempty"`
	Study      *StudyChapter `json:"study,omitempty"`
	lastPID    int
}

// Reference is a Bible citation or a publication reference of a part.
type Reference struct {
	Kind      string       `json:"kind"` // bible, publication
	Text      string       `json:"text"`
	Ref       string       `json:"reference,omitempty"`
	Range     *bible.Range `json:"range,omitempty"`
	DocID     int          `json:"docid,omitempty"`
	Pars      string       `json:"paragraphs,omitempty"`
	Location  string       `json:"location,omitempty"`
	Title     string       `json:"title,omitempty"`
	Pub       string       `json:"publication,omitempty"`
	InLibrary bool         `json:"in_library"`
	Sync      string       `json:"sync,omitempty"`
	Extract   string       `json:"extract,omitempty"`
	URL       string       `json:"url,omitempty"`
}

// Video mentioned by the meeting.
type Video struct {
	Key      string `json:"key"`
	Title    string `json:"title,omitempty"`
	Duration string `json:"duration,omitempty"`
	Part     string `json:"part,omitempty"`
	Command  string `json:"subtitles_command"`
}

// Image of the meeting material.
type Image struct {
	File    string `json:"file"`
	Alt     string `json:"alt,omitempty"`
	Caption string `json:"caption,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	DocID   int    `json:"docid"`
	Part    string `json:"part,omitempty"`
}

// StudyChapter is the chapter of the congregation Bible study, read from the
// extract the workbook ships (the whole chapter, questions included).
type StudyChapter struct {
	DocID      int             `json:"docid"`
	Pub        string          `json:"publication"`
	Location   string          `json:"location"`
	Label      string          `json:"chapter"` // the chapter number and name
	Title      string          `json:"title"`
	Accounts   []string        `json:"accounts,omitempty"`
	Groups     []QuestionGroup `json:"questions,omitempty"`
	References []Reference     `json:"references,omitempty"`
	Videos     []Video         `json:"videos,omitempty"`
	Images     []Image         `json:"images,omitempty"`
	Paragraphs int             `json:"paragraphs"`
}

// QuestionGroup is a heading together with the questions listed under it.
type QuestionGroup struct {
	Title     string   `json:"title"`
	Questions []string `json:"questions"`
}

// Watchtower is the study article of the week.
type Watchtower struct {
	DocRef
	Date      string          `json:"date"`
	Theme     string          `json:"theme_text,omitempty"`
	ThemeRef  string          `json:"theme_reference,omitempty"`
	Summary   string          `json:"theme,omitempty"`
	Songs     []Song          `json:"songs,omitempty"`
	Questions []StudyQuestion `json:"questions"`
	Boxes     []QuestionGroup `json:"boxes,omitempty"`
	Review    []string        `json:"review,omitempty"`
	Images    []Image         `json:"images,omitempty"`
	Footnotes []string        `json:"footnotes,omitempty"`
}

// StudyQuestion of a study article.
type StudyQuestion struct {
	Paragraphs string `json:"paragraphs"`
	Text       string `json:"text"`
	Subheading string `json:"subheading,omitempty"`
}

// Monday returns the Monday of the week that contains day.
func Monday(day time.Time) time.Time {
	wd := int(day.Weekday())
	if wd == 0 {
		wd = 7
	}
	return day.AddDate(0, 0, 1-wd)
}

// DateNum renders a date as the YYYYMMDD integers DatedText uses.
func DateNum(t time.Time) int {
	n, _ := strconv.Atoi(t.Format("20060102"))
	return n
}

// WorkbookIssue is the Meeting Workbook issue (every two months, odd month)
// that covers monday.
func WorkbookIssue(monday time.Time) string {
	y, m := monday.Year(), int(monday.Month())
	if m%2 == 0 {
		m--
	}
	return fmt.Sprintf("%04d%02d", y, m)
}

// WatchtowerIssues are the study-edition issues to try for a week. Since
// 2016 an issue is studied in the Mondays of the month two months later
// (w 2026-07 covers 7 Sep to 4 Oct 2026), so that month goes first; then the
// offsets Meeting Media Manager tries (6, 8, 10 and 12 weeks back).
func WatchtowerIssues(monday time.Time) []string {
	first := time.Date(monday.Year(), monday.Month(), 1, 0, 0, 0, 0, monday.Location()).AddDate(0, -2, 0)
	out := []string{first.Format("200601")}
	seen := map[string]bool{out[0]: true}
	for _, weeks := range []int{6, 8, 10, 12} {
		iss := monday.AddDate(0, 0, -7*weeks).Format("200601")
		if !seen[iss] {
			seen[iss] = true
			out = append(out, iss)
		}
	}
	return out
}

var (
	// Publications put no-break spaces between a number and its unit;
	// \s alone does not match them.
	// The digits a part uses to announce its length, inside whichever brackets the
	// script writes: ASCII, fullwidth CJK, or the lenticular brackets some use.
	parenRe = regexp.MustCompile(`[(\x{ff08}\x{3010}]([^)\x{ff09}\x{3011}]*)[)\x{ff09}\x{3011}]`)
)

// Builder assembles a week from the library.
type Builder struct {
	Store *store.Store
}

// BuildWorkbook parses the Meeting Workbook document of the week.
func (b *Builder) BuildWorkbook(w *Week, docid int, dated store.DatedDoc) error {
	d, err := b.Store.Doc(docid)
	if err != nil {
		return err
	}
	parsed, err := content.Parse(d.HTML)
	if err != nil {
		return err
	}
	extracts, err := b.Store.ExtractsOf(docid)
	if err != nil {
		return err
	}
	media, err := b.Store.MediaOf(docid)
	if err != nil {
		return err
	}
	loc, _, _ := strings.Cut(dated.Caption, " · ")
	w.Range = d.Title
	w.Workbook = &DocRef{DocID: docid, Pub: d.Pub.MepsSymbol, Key: d.Pub.Key, Title: d.Title, Location: loc, URL: content.WolDocURL(docid, 0)}

	byLink := map[string]store.Extract{}
	for _, e := range extracts {
		byLink[e.Link] = e
	}
	refFor := func(l *content.Link) Reference {
		r := Reference{Kind: "publication", Text: l.Text, DocID: l.DocID, Pars: l.Pars, URL: content.WolDocURL(l.DocID, l.First)}
		if e, ok := byLink[strings.TrimPrefix(l.Href, "jwpub://")]; ok {
			r.Location, r.Title, r.Pub = e.Caption, e.Title, e.RefSymbol
			r.Extract = extractText(e.HTML)
			r.Sync = syncHint(e)
		}
		if _, err := b.Store.Doc(l.DocID); err == nil {
			r.InLibrary = true
		}
		return r
	}

	var section *Section
	var part *Part
	songsSeen := 0
	flush := func() {
		if part != nil && section != nil {
			section.Parts = append(section.Parts, *part)
		}
		part = nil
	}
	ensureSection := func() {
		if section == nil {
			w.Sections = append(w.Sections, Section{})
			section = &w.Sections[len(w.Sections)-1]
		}
	}

	for _, it := range parsed.Items {
		if it.Image != nil {
			continue // placed below from DocumentMultimedia, which knows the paragraph
		}
		blk := it.Block
		text := blk.Text()
		switch {
		case blk.Kind == content.KindHeading && blk.Level == 1:
			continue
		case blk.Kind == content.KindHeading && blk.Level == 2 && len(blk.BibleRefs()) > 0 && w.WeeklyReading == nil:
			w.WeeklyReading = weeklyReading(text, blk.BibleRefs())
			continue
		case blk.Kind == content.KindHeading && blk.Level == 2:
			flush()
			w.Sections = append(w.Sections, Section{Title: text, Marker: blk.Marker})
			section = &w.Sections[len(w.Sections)-1]
			continue
		case blk.Kind == content.KindHeading && blk.Level == 3:
			flush()
			ensureSection()
			for _, s := range b.songsIn(blk, extracts, songsSeen) {
				w.Songs = append(w.Songs, s)
				songsSeen++
			}
			songTexts := make([]string, 0, 2)
			for _, l := range blk.PubLinks() {
				songTexts = append(songTexts, l.Text)
			}
			title := partTitle(text, songTexts)
			if title == "" {
				continue // a heading that only announces a song
			}
			part = &Part{Title: title, PID: blk.PID}
			if n, rest, ok := leadingNumber(title); ok {
				part.Number, part.Title = n, rest
			}
			if n, ok := minutesIn(text); ok {
				part.Minutes = n
			}
			continue
		}
		if part == nil {
			continue
		}
		if part.Minutes == 0 {
			if n, ok := minutesIn(text); ok {
				part.Minutes = n
			}
		}
		clean := strings.TrimSpace(stripMinutes(text))
		if blk.IsQuestion() {
			part.Questions = append(part.Questions, clean)
		} else if clean != "" && blk.Kind != content.KindCaption && blk.Kind != content.KindCredit {
			part.Text = append(part.Text, clean)
		}
		for _, l := range blk.Links {
			switch {
			case l.Kind == "bible" && l.Bible != nil:
				r := *l.Bible
				part.References = append(part.References, Reference{Kind: "bible", Text: l.Text, Ref: r.String(), Range: &r})
			case l.Kind == "pub" && l.DocID != 0 && l.DocID != docid:
				part.References = append(part.References, refFor(l))
			}
		}
		for _, v := range blk.Videos {
			vid := Video{Key: v.Key, Title: videoTitle(v.Title), Part: part.label(), Command: "pubkit subtitles " + v.Key}
			part.Videos = append(part.Videos, vid)
			w.Videos = append(w.Videos, vid)
		}
	}
	flush()

	// A part runs until the next heading. Captions carry high out-of-order
	// pids (figcaption p43 sits inside part 1), so the range cannot come from
	// the last block seen.
	var headings []int
	for _, blk := range parsed.Blocks {
		if blk.Kind == content.KindHeading {
			headings = append(headings, blk.PID)
		}
	}
	sort.Ints(headings)
	for si := range w.Sections {
		for pi := range w.Sections[si].Parts {
			p := &w.Sections[si].Parts[pi]
			p.lastPID = int(^uint(0) >> 1)
			for _, h := range headings {
				if h > p.PID {
					p.lastPID = h - 1
					break
				}
			}
		}
	}

	// Images: DocumentMultimedia says which paragraph each one belongs to.
	for _, m := range media {
		if m.DataType != 0 || m.BeginPID == 0 || !strings.Contains(m.File, "_cnt_") {
			continue
		}
		img := Image{File: m.File, Alt: m.Label, Caption: m.Caption, Width: m.Width, Height: m.Height, DocID: docid}
		if p := w.partAt(m.BeginPID); p != nil {
			img.Part = p.label()
			p.Images = append(p.Images, img)
		}
		w.Images = append(w.Images, img)
	}

	b.identifyParts(w, extracts)
	return nil
}

// Section markers, as publications name them in a class on the wrapper of each
// section heading. They are icons, not words, so they are the same in every
// language; the heading text beside them is not.
const (
	markerTreasures = "gem"       // the section that opens the meeting
	markerMinistry  = "wheat"     // the field-ministry section
	markerLiving    = "sheep"     // the closing section
	markerLegacyT   = "treasures" // the pre-2025 markup names them in words
	markerLegacyM   = "ministry"
	markerLegacyL   = "christianLiving"
)

func sectionRole(marker string) string {
	switch marker {
	case markerTreasures, markerLegacyT:
		return markerTreasures
	case markerMinistry, markerLegacyM:
		return markerMinistry
	case markerLiving, markerLegacyL:
		return markerLiving
	}
	return ""
}

// identifyParts works out which part is the student Bible reading and which is
// the congregation study WITHOUT reading their titles, because the titles are
// translated and the structure is not:
//
//   - the student reading is the last part of the opening section that carries a
//     Bible reference of its own;
//   - the congregation study is the last part of the closing section whose
//     publication extract is a book chapter rather than a song, identified by
//     the extract parsing successfully rather than by what the part is called.
//
// When a signal is missing the field is left empty and the reason is recorded,
// which is the honest outcome for the weeks that genuinely have no such part
// (an assembly, a circuit overseer's visit).
func (b *Builder) identifyParts(w *Week, extracts []store.Extract) {
	marked := false
	for _, sec := range w.Sections {
		if sectionRole(sec.Marker) != "" {
			marked = true
		}
	}
	if !marked {
		// Not a language problem: this markup does not mark its sections at all,
		// so nothing can be derived from which section a part sits in. The parts
		// and their text are still there.
		w.Notes = append(w.Notes, "this workbook's markup carries no section markers, so the student reading and the congregation study could not be identified")
		return
	}
	for si := range w.Sections {
		role := sectionRole(w.Sections[si].Marker)
		parts := w.Sections[si].Parts
		for pi := len(parts) - 1; pi >= 0; pi-- {
			p := &w.Sections[si].Parts[pi]
			switch role {
			case markerTreasures:
				if w.StudentReading == nil {
					if a := studentReading(p); a.Range != nil {
						w.StudentReading = a
					}
				}
			case markerLiving:
				if w.CongregationStudy == nil {
					b.attachStudy(w, p, extracts)
				}
			}
		}
	}
	if w.StudentReading == nil {
		w.Notes = append(w.Notes, "no part of the opening section carries a Bible reference of its own, so there is no student reading to report")
	}
	if w.CongregationStudy == nil {
		w.Notes = append(w.Notes, "no part of the closing section extracts a book chapter, so there is no congregation study to report")
	}
}

// attachStudy tries to read a part's publication reference as a study chapter.
// The successful parse IS the identification: a part that yields a chapter with
// its questions is the congregation study, whatever it is called.
func (b *Builder) attachStudy(w *Week, p *Part, extracts []store.Extract) {
	for _, r := range p.References {
		if r.Kind != "publication" || r.DocID == 0 {
			continue
		}
		for _, e := range extracts {
			if e.RefDocID != r.DocID || e.RefClass == songClass {
				continue
			}
			sc, err := studyChapter(e)
			if err != nil || sc == nil {
				continue
			}
			p.Study = sc
			w.CongregationStudy = sc
			for _, v := range sc.Videos {
				v.Part = p.label()
				w.Videos = append(w.Videos, v)
			}
			for _, im := range sc.Images {
				im.Part = p.label()
				w.Images = append(w.Images, im)
			}
			return
		}
	}
}

// songClass is the document class a publication gives a songbook entry in its
// extract table. Measured identical across languages; the symbol beside it
// ("sjj") is NOT — it is translated — so the class is what can be matched on.
const songClass = 31

// whenOf names a song by its position in the meeting, which is the only
// language-neutral thing about it.
func whenOf(i int) string {
	switch i {
	case 0:
		return "start"
	case 1:
		return "middle"
	default:
		return "end"
	}
}

// songsIn finds the songs a heading announces without reading the heading. A
// song is a publication extract of class 31 anchored at this heading; its number
// is the chapter number of the songbook document it points at, which is a number
// in every language. Only if the songbook is not in the library does it fall
// back to reading digits out of the caption.
func (b *Builder) songsIn(blk *content.Block, extracts []store.Extract, seen int) []Song {
	var out []Song
	add := func(docid int, title, caption string) {
		s := Song{DocID: docid, Title: title, When: whenOf(seen + len(out))}
		if n, ok := b.chapterNumber(docid); ok {
			s.Number = n
		} else if n, ok := firstNumber(caption); ok {
			s.Number = n
		}
		if s.Number > 0 {
			s.Video = fmt.Sprintf("pub-sjjm_%d_VIDEO", s.Number)
		}
		out = append(out, s)
	}
	for _, e := range extracts {
		if e.RefClass == songClass && e.BeginPID == blk.PID {
			add(e.RefDocID, e.Title, e.Caption)
		}
	}
	if len(out) > 0 {
		return out
	}
	// A heading can link the song without the workbook shipping an extract of it.
	for _, l := range blk.PubLinks() {
		if l.DocID == 0 {
			continue
		}
		if n, ok := b.chapterNumber(l.DocID); ok && isSongDoc(b, l.DocID) {
			out = append(out, Song{DocID: l.DocID, Number: n, When: whenOf(seen + len(out)),
				Video: fmt.Sprintf("pub-sjjm_%d_VIDEO", n)})
		}
	}
	return out
}

func (b *Builder) chapterNumber(docid int) (int, bool) {
	if b.Store == nil || docid == 0 {
		return 0, false
	}
	n, ok := b.Store.ChapterNumber(docid)
	return n, ok && n > 0
}

// isSongDoc asks the library whether a document belongs to the songbook, by the
// class the publication itself records — never by its symbol, which is translated.
func isSongDoc(b *Builder, docid int) bool {
	if b.Store == nil {
		return false
	}
	d, err := b.Store.Doc(docid)
	return err == nil && d.Class == songClass
}

// minutesIn reads the length a part announces. The digits can be of any script;
// what marks them is the parentheses the publication puts them in, which every
// language keeps.
func minutesIn(text string) (int, bool) {
	m := parenRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	return firstNumber(m[1])
}

func (p *Part) label() string {
	if p.Number > 0 {
		return fmt.Sprintf("%d. %s", p.Number, p.Title)
	}
	return p.Title
}

func (w *Week) partAt(pid int) *Part {
	for si := range w.Sections {
		for pi := range w.Sections[si].Parts {
			p := &w.Sections[si].Parts[pi]
			if pid >= p.PID && pid <= p.lastPID {
				return p
			}
		}
	}
	return nil
}

// partTitle is what is left of a heading once the song announcement and the
// length are taken out of it. A heading can announce a song and a part at once,
// separated by a bar; the song is recognized by the text of its own link, not by
// the word for "song", which is translated.
func partTitle(h string, songTexts []string) string {
	var keep []string
	for _, piece := range strings.Split(h, "|") {
		piece = strings.TrimSpace(stripMinutes(piece))
		if piece == "" || isSongPiece(piece, songTexts) {
			continue
		}
		keep = append(keep, piece)
	}
	return strings.Join(keep, " | ")
}

// isSongPiece reports whether a piece of a heading is the song announcement,
// which it is when the song's own link text is most of what the piece says.
func isSongPiece(piece string, songTexts []string) bool {
	for _, t := range songTexts {
		t = strings.TrimSpace(t)
		if t == "" || !strings.Contains(piece, t) {
			continue
		}
		// The rest is a connecting word or two ("and prayer"), not a part.
		if len([]rune(piece))-len([]rune(t)) <= 24 {
			return true
		}
	}
	return false
}

// stripMinutes removes the parenthetical a part uses to announce its length,
// and only that one: a parenthetical holding a number and little else. Removing
// every parenthetical would delete text the publication meant to show.
func stripMinutes(text string) string {
	for _, m := range parenRe.FindAllStringSubmatch(text, -1) {
		if _, ok := firstNumber(m[1]); !ok {
			continue
		}
		if len([]rune(m[1])) > 16 {
			continue
		}
		text = strings.Replace(text, m[0], "", 1)
	}
	return text
}

func weeklyReading(text string, refs []bible.Range) *Reading {
	r := &Reading{Text: text, Ranges: refs}
	if len(refs) > 0 {
		bk, _ := bible.BookByNum(refs[0].Book)
		r.Book, r.BookNum = bk.Name, bk.Num
		for _, rg := range refs {
			for c := rg.StartChapter; c <= rg.EndChapter; c++ {
				if len(r.Chapters) == 0 || r.Chapters[len(r.Chapters)-1] != c {
					r.Chapters = append(r.Chapters, c)
				}
			}
		}
		r.Ref = bible.FormatList(refs)
	}
	return r
}

func studentReading(p *Part) *Assignment {
	a := &Assignment{}
	for i := range p.References {
		r := &p.References[i]
		switch {
		case r.Kind == "bible" && a.Range == nil:
			a.Ref, a.Range = r.Ref, r.Range
		case r.Kind == "publication" && a.Lesson == nil:
			a.Lesson = r
		}
	}
	return a
}

// videoTitle drops the generic "Ponga el VIDEO" link text.
func videoTitle(t string) string {
	t = strings.TrimSpace(t)
	if strings.EqualFold(strings.TrimSuffix(t, "."), "Ponga el VIDEO") || strings.EqualFold(t, "VIDEO") {
		return ""
	}
	return t
}

func extractText(h string) string {
	if h == "" {
		return ""
	}
	d, err := content.Parse(h)
	if err != nil {
		return content.InnerText(h)
	}
	var parts []string
	for _, b := range d.Blocks {
		t := b.Text()
		if t == "" || b.Kind == content.KindCaption || b.Kind == content.KindCredit {
			continue
		}
		if b.Num > 0 && b.Kind == content.KindPara {
			t = fmt.Sprintf("%d %s", b.Num, t)
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, "\n")
}

// syncHint is the command that brings the referenced publication.
func syncHint(e store.Extract) string {
	sym := e.RefUndated
	if sym == "" {
		sym = strings.TrimRight(e.RefSymbol, "0123456789")
	}
	if sym == "" {
		return ""
	}
	if e.RefIssue != 0 {
		return fmt.Sprintf("pubkit sync %s --issue %s", sym, store.NormalizeIssue(strconv.Itoa(e.RefIssue)))
	}
	return "pubkit sync " + sym
}

// studyChapter reads the congregation study chapter from the extract.
func studyChapter(e store.Extract) (*StudyChapter, error) {
	d, err := content.Parse(e.HTML)
	if err != nil {
		return nil, err
	}
	sc := &StudyChapter{DocID: e.RefDocID, Pub: e.RefSymbol, Location: e.Caption, Title: e.Title}
	var group *QuestionGroup
	inAccounts := false
	for i, it := range d.Items {
		if img := it.Image; img != nil {
			if img.Height > 0 && img.Height < 200 {
				continue // decorative strip
			}
			sc.Images = append(sc.Images, Image{File: img.File, Alt: img.Alt, Caption: img.Caption, Width: img.Width, Height: img.Height, DocID: e.RefDocID})
			continue
		}
		blk := it.Block
		text := blk.Text()
		switch {
		case blk.Kind == content.KindContext:
			sc.Label = text
		case blk.Kind == content.KindHeading && blk.Level == 1:
			sc.Title = text
		case blk.Kind == content.KindHeading:
			// The accounts to read are the group that is nothing but Bible
			// references: no question, no answer field. That shape is the same
			// in every language, unlike the heading that announces it.
			inAccounts = onlyBibleRefs(d.Items, i)
			sc.Groups = append(sc.Groups, QuestionGroup{Title: text})
			group = &sc.Groups[len(sc.Groups)-1]
		case blk.Kind == content.KindPara && !blk.IsQuestion() && group == nil:
			sc.Paragraphs++
		}
		if inAccounts {
			for _, rg := range blk.BibleRefs() {
				sc.Accounts = append(sc.Accounts, rg.Long())
			}
		}
		if blk.IsQuestion() && group != nil {
			group.Questions = append(group.Questions, text)
		}
		for _, l := range blk.Links {
			if l.Kind == "pub" && l.DocID != 0 && l.DocID != e.RefDocID {
				sc.References = append(sc.References, Reference{Kind: "publication", Text: l.Text, DocID: l.DocID, Pars: l.Pars,
					URL: content.WolDocURL(l.DocID, l.First)})
			}
		}
		for _, v := range blk.Videos {
			sc.Videos = append(sc.Videos, Video{Key: v.Key, Title: videoTitle(v.Title), Command: "pubkit subtitles " + v.Key})
		}
	}
	// Keep only groups that ask something.
	var groups []QuestionGroup
	for _, g := range sc.Groups {
		if len(g.Questions) > 0 {
			groups = append(groups, g)
		}
	}
	sc.Groups = groups
	sc.Videos = dedupeVideos(sc.Videos)
	return sc, nil
}

func dedupeVideos(vs []Video) []Video {
	var out []Video
	idx := map[string]int{}
	for _, v := range vs {
		if i, ok := idx[v.Key]; ok {
			if out[i].Title == "" {
				out[i].Title = v.Title
			}
			continue
		}
		idx[v.Key] = len(out)
		out = append(out, v)
	}
	return out
}

// BuildWatchtower parses the study article.
func (b *Builder) BuildWatchtower(w *Week, docid int) error {
	d, err := b.Store.Doc(docid)
	if err != nil {
		return err
	}
	parsed, err := content.Parse(d.HTML)
	if err != nil {
		return err
	}
	wt := &Watchtower{DocRef: DocRef{DocID: docid, Pub: d.Pub.MepsSymbol, Key: d.Pub.Key, Title: d.Title, URL: content.WolDocURL(docid, 0)}}
	if d.Pub.Issue != "" {
		wt.Location = fmt.Sprintf("%s (%s)", d.Pub.MepsSymbol, d.Pub.Issue)
	}
	subheading := ""
	expectSummary := false
	var box *QuestionGroup
	for _, it := range parsed.Items {
		if img := it.Image; img != nil {
			wt.Images = append(wt.Images, Image{File: img.File, Alt: img.Alt, Caption: img.Caption, Width: img.Width, Height: img.Height, DocID: docid})
			continue
		}
		blk := it.Block
		text := blk.Text()
		if !blk.InBox {
			box = nil
		}
		switch blk.Kind {
		case content.KindContext:
			wt.Date = text
		case content.KindMeta:
			// A song announcement is a link into the songbook. Its number comes
			// from the linked document, or from the digits beside it in whatever
			// script the publication uses — never from the word "song".
			if links := blk.PubLinks(); len(links) > 0 {
				if n, ok := b.songNumber(links[0].DocID, text); ok {
					when := "start"
					if len(wt.Songs) > 0 {
						when = "end"
					}
					wt.Songs = append(wt.Songs, Song{Number: n, Title: songTitle(text, links[0].Text),
						DocID: links[0].DocID, When: when, Video: fmt.Sprintf("pub-sjjm_%d_VIDEO", n)})
					continue
				}
			}
			if isLabel(text) {
				expectSummary = true
				continue
			}
			if expectSummary {
				wt.Summary = text
				expectSummary = false
			}
		case content.KindTheme:
			wt.Theme = text
			if refs := blk.BibleRefs(); len(refs) > 0 {
				wt.ThemeRef = bible.FormatList(refs)
			}
		case content.KindHeading:
			if blk.InBox {
				wt.Boxes = append(wt.Boxes, QuestionGroup{Title: text})
				box = &wt.Boxes[len(wt.Boxes)-1]
				continue
			}
			if blk.Level == 1 {
				wt.Title = text
				continue
			}
			subheading = text
		case content.KindQuestion:
			label := blk.NumLabel
			if label == "" && blk.Num > 0 {
				label = strconv.Itoa(blk.Num)
			}
			wt.Questions = append(wt.Questions, StudyQuestion{Paragraphs: label, Text: text, Subheading: subheading})
		case content.KindFootnote:
			fn := text
			if blk.FnLabel != "" {
				fn = blk.FnLabel + ") " + text
			}
			wt.Footnotes = append(wt.Footnotes, fn)
		default:
			if box != nil && text != "" {
				box.Questions = append(box.Questions, text)
				if blk.Answer {
					wt.Review = append(wt.Review, text)
				}
			}
		}
	}
	w.Watchtower = wt
	return nil
}

func isLabel(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && len([]rune(s)) <= 12 && strings.ToUpper(s) == s
}

// SortedVideoKeys lists unique video keys (without songs) for title lookups.
func (w *Week) SortedVideoKeys() []string {
	seen := map[string]bool{}
	var keys []string
	for _, v := range w.Videos {
		if !seen[v.Key] {
			seen[v.Key] = true
			keys = append(keys, v.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// SetVideoTitle fills title and duration wherever a video appears.
func (w *Week) SetVideoTitle(key, title, duration string) {
	fill := func(v *Video) {
		if v.Key == key {
			if title != "" {
				v.Title = title
			}
			v.Duration = duration
		}
	}
	for i := range w.Videos {
		fill(&w.Videos[i])
	}
	for si := range w.Sections {
		for pi := range w.Sections[si].Parts {
			p := &w.Sections[si].Parts[pi]
			for i := range p.Videos {
				fill(&p.Videos[i])
			}
			if p.Study != nil {
				for i := range p.Study.Videos {
					fill(&p.Study.Videos[i])
				}
			}
		}
	}
}

// FindWorkbook returns the DatedText row of the Meeting Workbook for monday.
func (b *Builder) FindWorkbook(monday time.Time) (*store.DatedDoc, error) {
	docs, err := b.Store.DatedDocs("mwb", DateNum(monday))
	if err != nil || len(docs) == 0 {
		return nil, err
	}
	return &docs[0], nil
}

var datedLinkRe = regexp.MustCompile(`p/[A-Z]+:(\d+)/(\d+)(?:-(\d+))?`)

// FindWatchtower returns the study article for the week that starts on
// monday. The DatedText row points to the table of contents (e.g.
// "p/S:2026481/9-10"); the link in those paragraphs is the article. If that
// fails, the n-th dated week is the n-th study article, as M³ does.
func (b *Builder) FindWatchtower(monday time.Time) (int, error) {
	docs, err := b.Store.DatedDocs("w", DateNum(monday))
	if err != nil {
		return 0, err
	}
	for _, dd := range docs {
		if dd.First != DateNum(monday) {
			continue
		}
		if m := datedLinkRe.FindStringSubmatch(dd.Link); m != nil {
			toc, _ := strconv.Atoi(m[1])
			from, _ := strconv.Atoi(m[2])
			to := from
			if m[3] != "" {
				to, _ = strconv.Atoi(m[3])
			}
			if d, err := b.Store.Doc(toc); err == nil {
				if parsed, err := content.Parse(d.HTML); err == nil {
					for _, blk := range parsed.Blocks {
						if blk.PID < from || blk.PID > to {
							continue
						}
						for _, l := range blk.PubLinks() {
							if l.DocID != 0 && l.DocID != toc {
								return l.DocID, nil
							}
						}
					}
				}
			}
		}
		rank, err := b.Store.DatedRank(dd.PubKey, dd.First)
		if err != nil {
			return 0, err
		}
		arts, err := b.Store.StudyArticles(dd.PubKey)
		if err != nil {
			return 0, err
		}
		if rank < len(arts) {
			return arts[rank], nil
		}
	}
	return 0, nil
}

// Normalize replaces nil lists with empty ones so JSON consumers (jq) always
// get arrays.
func (w *Week) Normalize() {
	if w.Songs == nil {
		w.Songs = []Song{}
	}
	if w.Sections == nil {
		w.Sections = []Section{}
	}
	if w.Videos == nil {
		w.Videos = []Video{}
	}
	if w.Images == nil {
		w.Images = []Image{}
	}
	for si := range w.Sections {
		if w.Sections[si].Parts == nil {
			w.Sections[si].Parts = []Part{}
		}
	}
	if wt := w.Watchtower; wt != nil && wt.Questions == nil {
		wt.Questions = []StudyQuestion{}
	}
}

// onlyBibleRefs reports whether the blocks under the heading at index i carry
// Bible references and nothing that asks the reader anything. That is the shape
// of the "read these accounts" group, in any language.
func onlyBibleRefs(items []content.Item, i int) bool {
	refs := false
	for _, it := range items[i+1:] {
		blk := it.Block
		if blk == nil {
			continue
		}
		if blk.Kind == content.KindHeading {
			break
		}
		if blk.IsQuestion() || blk.Answer {
			return false
		}
		if len(blk.BibleRefs()) > 0 {
			refs = true
		}
	}
	return refs
}

// songNumber resolves the number of the song a link points at: from the songbook
// document when the library holds it, otherwise from the digits printed beside
// the link. It reports false when the link is not a song at all.
func (b *Builder) songNumber(docid int, text string) (int, bool) {
	if docid == 0 {
		return 0, false
	}
	if n, ok := b.chapterNumber(docid); ok && isSongDoc(b, docid) {
		return n, true
	}
	if b.Store != nil {
		if d, err := b.Store.Doc(docid); err == nil && d.Class != songClass {
			return 0, false
		}
	}
	return firstNumber(text)
}

// songTitle is whatever the announcement says once the link itself is taken out
// of it, which is the part the publication wrote about this particular song.
func songTitle(text, linkText string) string {
	return strings.TrimSpace(strings.Replace(text, linkText, "", 1))
}
