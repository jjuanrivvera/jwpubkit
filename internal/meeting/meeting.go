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

// Week is everything `pubkit semana` reports.
type Week struct {
	Monday            string        `json:"lunes"`
	Range             string        `json:"rango"`
	Workbook          *DocRef       `json:"guia"`
	WeeklyReading     *Reading      `json:"lectura_semanal,omitempty"`
	StudentReading    *Assignment   `json:"lectura_estudiante,omitempty"`
	Songs             []Song        `json:"canciones"`
	Sections          []Section     `json:"secciones"`
	Videos            []Video       `json:"videos"`
	Images            []Image       `json:"imagenes"`
	Watchtower        *Watchtower   `json:"atalaya,omitempty"`
	Notes             []string      `json:"avisos,omitempty"`
	CongregationStudy *StudyChapter `json:"-"`
}

// DocRef identifies a document.
type DocRef struct {
	DocID    int    `json:"docid"`
	Pub      string `json:"publicacion"`
	Key      string `json:"clave"`
	Title    string `json:"titulo"`
	Location string `json:"ubicacion,omitempty"`
	URL      string `json:"url"`
}

// Reading is the weekly Bible reading.
type Reading struct {
	Text     string        `json:"texto"`
	Book     string        `json:"libro"`
	BookNum  int           `json:"libro_num"`
	Chapters []int         `json:"capitulos"`
	Ranges   []bible.Range `json:"rangos"`
	Ref      string        `json:"cita"`
}

// Assignment is the student Bible reading.
type Assignment struct {
	Ref    string       `json:"cita"`
	Range  *bible.Range `json:"rango,omitempty"`
	Lesson *Reference   `json:"leccion,omitempty"`
}

// Song of the meeting.
type Song struct {
	Number int    `json:"numero"`
	Title  string `json:"titulo,omitempty"`
	DocID  int    `json:"docid,omitempty"`
	When   string `json:"momento"` // inicio, medio, final
	Video  string `json:"video"`
}

// Section groups the parts under one of the workbook's top-level headings.
type Section struct {
	Title string `json:"titulo"`
	Parts []Part `json:"partes"`
}

// Part of the meeting.
type Part struct {
	Number     int           `json:"numero,omitempty"`
	Title      string        `json:"titulo"`
	Minutes    int           `json:"minutos,omitempty"`
	PID        int           `json:"pid"`
	Text       []string      `json:"texto,omitempty"`
	Questions  []string      `json:"preguntas,omitempty"`
	References []Reference   `json:"referencias,omitempty"`
	Videos     []Video       `json:"videos,omitempty"`
	Images     []Image       `json:"imagenes,omitempty"`
	Study      *StudyChapter `json:"estudio,omitempty"`
	lastPID    int
}

// Reference is a Bible citation or a publication reference of a part.
type Reference struct {
	Kind      string       `json:"tipo"` // biblia, publicacion
	Text      string       `json:"texto"`
	Ref       string       `json:"cita,omitempty"`
	Range     *bible.Range `json:"rango,omitempty"`
	DocID     int          `json:"docid,omitempty"`
	Pars      string       `json:"parrafos,omitempty"`
	Location  string       `json:"ubicacion,omitempty"`
	Title     string       `json:"titulo,omitempty"`
	Pub       string       `json:"publicacion,omitempty"`
	InLibrary bool         `json:"en_biblioteca"`
	Sync      string       `json:"sync,omitempty"`
	Extract   string       `json:"extracto,omitempty"`
	URL       string       `json:"url,omitempty"`
}

// Video mentioned by the meeting.
type Video struct {
	Key      string `json:"clave"`
	Title    string `json:"titulo,omitempty"`
	Duration string `json:"duracion,omitempty"`
	Part     string `json:"parte,omitempty"`
	Command  string `json:"subtitulos"`
}

// Image of the meeting material.
type Image struct {
	File    string `json:"archivo"`
	Alt     string `json:"alt,omitempty"`
	Caption string `json:"pie,omitempty"`
	Width   int    `json:"ancho,omitempty"`
	Height  int    `json:"alto,omitempty"`
	DocID   int    `json:"docid"`
	Part    string `json:"parte,omitempty"`
}

// StudyChapter is the chapter of the congregation Bible study, read from the
// extract the workbook ships (the whole chapter, questions included).
type StudyChapter struct {
	DocID      int             `json:"docid"`
	Pub        string          `json:"publicacion"`
	Location   string          `json:"ubicacion"`
	Label      string          `json:"capitulo"` // "10 MOISÉS"
	Title      string          `json:"titulo"`
	Accounts   []string        `json:"relatos_biblicos,omitempty"`
	Groups     []QuestionGroup `json:"preguntas,omitempty"`
	References []Reference     `json:"referencias,omitempty"`
	Videos     []Video         `json:"videos,omitempty"`
	Images     []Image         `json:"imagenes,omitempty"`
	Paragraphs int             `json:"parrafos"`
}

// QuestionGroup is a heading together with the questions listed under it.
type QuestionGroup struct {
	Title     string   `json:"titulo"`
	Questions []string `json:"preguntas"`
}

// Watchtower is the study article of the week.
type Watchtower struct {
	DocRef
	Date      string          `json:"fecha"`
	Theme     string          `json:"texto_tematico,omitempty"`
	ThemeRef  string          `json:"cita_tematica,omitempty"`
	Summary   string          `json:"tema,omitempty"`
	Songs     []Song          `json:"canciones,omitempty"`
	Questions []StudyQuestion `json:"preguntas"`
	Boxes     []QuestionGroup `json:"recuadros,omitempty"`
	Review    []string        `json:"repaso,omitempty"`
	Images    []Image         `json:"imagenes,omitempty"`
	Footnotes []string        `json:"notas,omitempty"`
}

// StudyQuestion of a study article.
type StudyQuestion struct {
	Paragraphs string `json:"parrafos"`
	Text       string `json:"texto"`
	Subheading string `json:"subtitulo,omitempty"`
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
	// Publications put no-break spaces in "Canción\u00a0128" and "(4\u00a0mins.)";
	// \s alone does not match them.
	minutesRe = regexp.MustCompile(`\((\d+)[\s\x{a0}]*mins?\.\)`)
	songRe    = regexp.MustCompile(`(?i)canci[oó]n[\s\x{a0}]+(\d+)`)
	partNumRe = regexp.MustCompile(`^(\d+)\.[\s\x{a0}]*(.+)$`)
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
		r := Reference{Kind: "publicacion", Text: l.Text, DocID: l.DocID, Pars: l.Pars, URL: content.WolDocURL(l.DocID, l.First)}
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
	whenOf := func(i int) string {
		switch i {
		case 0:
			return "inicio"
		case 1:
			return "medio"
		default:
			return "final"
		}
	}
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
			w.Sections = append(w.Sections, Section{Title: text})
			section = &w.Sections[len(w.Sections)-1]
			continue
		case blk.Kind == content.KindHeading && blk.Level == 3:
			flush()
			ensureSection()
			for _, m := range songRe.FindAllStringSubmatch(text, -1) {
				n, _ := strconv.Atoi(m[1])
				s := Song{Number: n, When: whenOf(songsSeen), Video: fmt.Sprintf("pub-sjjm_%d_VIDEO", n)}
				s.DocID, s.Title = songDoc(n, blk.PubLinks(), extracts)
				w.Songs = append(w.Songs, s)
				songsSeen++
			}
			title := partTitle(text)
			if title == "" {
				continue // a heading that only announces a song ("Canción 90")
			}
			part = &Part{Title: title, PID: blk.PID}
			if m := partNumRe.FindStringSubmatch(title); m != nil {
				part.Number, _ = strconv.Atoi(m[1])
				part.Title = m[2]
			}
			if m := minutesRe.FindStringSubmatch(text); m != nil {
				part.Minutes, _ = strconv.Atoi(m[1])
			}
			continue
		}
		if part == nil {
			continue
		}
		if part.Minutes == 0 {
			if m := minutesRe.FindStringSubmatch(text); m != nil {
				part.Minutes, _ = strconv.Atoi(m[1])
			}
		}
		clean := strings.TrimSpace(minutesRe.ReplaceAllString(text, ""))
		if blk.IsQuestion() {
			part.Questions = append(part.Questions, clean)
		} else if clean != "" && blk.Kind != content.KindCaption && blk.Kind != content.KindCredit {
			part.Text = append(part.Text, clean)
		}
		for _, l := range blk.Links {
			switch {
			case l.Kind == "biblia" && l.Bible != nil:
				r := *l.Bible
				part.References = append(part.References, Reference{Kind: "biblia", Text: l.Text, Ref: r.String(), Range: &r})
			case l.Kind == "pub" && l.DocID != 0 && l.DocID != docid:
				part.References = append(part.References, refFor(l))
			}
		}
		for _, v := range blk.Videos {
			vid := Video{Key: v.Key, Title: videoTitle(v.Title), Part: part.label(), Command: "pubkit subtitulos " + v.Key}
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

	for si := range w.Sections {
		for pi := range w.Sections[si].Parts {
			p := &w.Sections[si].Parts[pi]
			low := strings.ToLower(p.Title)
			switch {
			case strings.Contains(low, "lectura de la biblia"):
				w.StudentReading = studentReading(p)
			case strings.Contains(low, "estudio bíblico de la congregación"):
				for _, r := range p.References {
					if r.Kind != "publicacion" {
						continue
					}
					for _, e := range extracts {
						if e.RefDocID == r.DocID {
							sc, err := studyChapter(e)
							if err == nil {
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
							}
							break
						}
					}
					break
				}
			}
		}
	}
	return nil
}

// songDoc finds the docid and title of song n from the heading links or,
// failing that, from the extracts ("sjj canción 128").
func songDoc(n int, links []*content.Link, extracts []store.Extract) (int, string) {
	num := strconv.Itoa(n)
	for _, l := range links {
		if m := songRe.FindStringSubmatch(l.Text); m != nil && m[1] == num {
			for _, e := range extracts {
				if e.RefDocID == l.DocID {
					return l.DocID, e.Title
				}
			}
			return l.DocID, ""
		}
	}
	for _, e := range extracts {
		if m := songRe.FindStringSubmatch(e.Caption); m != nil && m[1] == num && strings.HasPrefix(e.Caption, "sjj") {
			return e.RefDocID, e.Title
		}
	}
	return 0, ""
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

// partTitle drops the song announcement from a heading:
// "Canción 102 y oración | Título de la parte (1 min.)" → "Título de la parte".
func partTitle(h string) string {
	var keep []string
	for _, piece := range strings.Split(h, "|") {
		piece = strings.TrimSpace(minutesRe.ReplaceAllString(piece, ""))
		if piece == "" || songRe.MatchString(piece) {
			continue
		}
		keep = append(keep, piece)
	}
	return strings.Join(keep, " | ")
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
		case r.Kind == "biblia" && a.Range == nil:
			a.Ref, a.Range = r.Ref, r.Range
		case r.Kind == "publicacion" && a.Lesson == nil:
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
	for _, it := range d.Items {
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
			inAccounts = strings.Contains(strings.ToLower(text), "relato bíblico")
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
				sc.References = append(sc.References, Reference{Kind: "publicacion", Text: l.Text, DocID: l.DocID, Pars: l.Pars,
					URL: content.WolDocURL(l.DocID, l.First)})
			}
		}
		for _, v := range blk.Videos {
			sc.Videos = append(sc.Videos, Video{Key: v.Key, Title: videoTitle(v.Title), Command: "pubkit subtitulos " + v.Key})
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
			if len(blk.PubLinks()) > 0 && songRe.MatchString(text) {
				m := songRe.FindStringSubmatch(text)
				n, _ := strconv.Atoi(m[1])
				when := "inicio"
				if len(wt.Songs) > 0 {
					when = "final"
				}
				title := strings.TrimSpace(songRe.ReplaceAllString(text, ""))
				wt.Songs = append(wt.Songs, Song{Number: n, Title: title, DocID: blk.PubLinks()[0].DocID, When: when, Video: fmt.Sprintf("pub-sjjm_%d_VIDEO", n)})
				continue
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
