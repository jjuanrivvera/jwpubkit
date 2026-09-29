// Package content turns the decrypted HTML of a JWPUB document into ordered
// blocks (paragraphs with their data-pid, headings, questions, captions,
// footnotes), images and links, and renders them as Markdown or plain text.
package content

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
)

// Block kinds.
const (
	KindHeading    = "h"       // Level says which
	KindPara       = "p"       // body paragraph
	KindItem       = "li"      // list item
	KindQuestion   = "q"       // study question (p.qu)
	KindTheme      = "theme"   // theme scripture (p.themeScrp)
	KindContext    = "context" // p.contextTtl: date or chapter label above the title
	KindMeta       = "meta"    // p.pubRefs: song line, "TEMA" label
	KindCaption    = "caption" // figcaption paragraph
	KindCredit     = "credit"  // image credit line
	KindFootnote   = "fn"      // footnote body
	KindDefinition = "def"     // a glossary entry: Term is the word, Text the definition
	KindOther      = "other"
)

// Link is an anchor inside a block.
type Link struct {
	Kind  string       `json:"kind"` // biblia, pub, interno, video, web
	Text  string       `json:"text"`
	Href  string       `json:"href"`
	Bible *bible.Range `json:"biblia,omitempty"`
	DocID int          `json:"docid,omitempty"`
	Pars  string       `json:"paragraphs,omitempty"` // "22-22", "17-17:159"
	First int          `json:"first_paragraph,omitempty"`
	Last  int          `json:"last_paragraph,omitempty"`
	Video *VideoRef    `json:"video,omitempty"`
}

// VideoRef points to a video in the mediator catalog.
type VideoRef struct {
	Key   string `json:"key"` // "pub-jwb-125_4_VIDEO"
	Pub   string `json:"pub,omitempty"`
	Issue string `json:"issue,omitempty"`
	Track int    `json:"track,omitempty"`
	DocID int    `json:"docid,omitempty"`
	Title string `json:"title,omitempty"`
}

// Image is a figure of the document.
type Image struct {
	File    string    `json:"file"`
	Alt     string    `json:"alt,omitempty"`
	Caption string    `json:"caption,omitempty"`
	Credit  string    `json:"credit,omitempty"`
	Width   int       `json:"width,omitempty"`
	Height  int       `json:"height,omitempty"`
	PID     int       `json:"pid,omitempty"` // block right before the figure
	Video   *VideoRef `json:"video,omitempty"`
}

// Run is a styled piece of text inside a block.
type Run struct {
	Text   string
	Bold   bool
	Italic bool
	Link   *Link
	FnMark string // footnote call ("a"); Text is empty
}

// Block is one element carrying a data-pid.
type Block struct {
	PID     int
	Kind    string
	Level   int      // heading level (1-4)
	Num     int      // paragraph number as publications cite it, or question number
	Sub     int      // numbered subentry of an encyclopedia article
	Class   string   // first class of the element: "sb", "sn", "qu"...
	Classes []string // every class of the element
	// Marker is the untranslated icon name a publication puts on the wrapper of
	// a section heading ("gem", "wheat", "sheep", "music"). It is the same in
	// every language, which is what lets a section be recognized without reading
	// the words in it.
	Marker string
	// Term is set on a glossary entry: the word being defined, taken from the
	// untranslated markup that marks it, with Text carrying the definition.
	Term       string
	definition string
	NumLabel   string // "3, 4" for questions that cover two paragraphs
	FnLabel    string // footnote letter
	RelPID     int    // question this paragraph answers (data-rel-pid)
	Answer     bool   // followed by an answer box: the text is a prompt
	InBox      bool   // inside a box / aside
	Runs       []Run
	Links      []*Link
	Videos     []VideoRef
}

// Item keeps blocks and figures in document order.
type Item struct {
	Block *Block
	Image *Image
}

// Doc is a parsed document.
type Doc struct {
	Items  []Item
	Blocks []*Block
	Images []*Image
	Videos []VideoRef
}

// Parse reads the HTML fragment stored in Document.Content (or Extract.Content).
func Parse(src string) (*Doc, error) {
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return nil, err
	}
	p := &parser{doc: &Doc{}}
	for _, n := range nodes {
		p.walk(n, style{})
	}
	for _, b := range p.doc.Blocks {
		b.finish()
	}
	p.doc.numberSubentries()
	return p.doc, nil
}

type style struct {
	marker       string
	bold, italic bool
	link         *Link
	inBox        bool
	inFootnote   bool
	inCaption    bool
	inItem       bool
}

type parser struct {
	doc     *Doc
	cur     *Block
	last    *Block
	figure  *Image
	figDone bool
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// iconMarker reads the untranslated icon a publication attaches to a section
// heading. Two markup generations are in the wild and both name the icon in a
// class rather than in words:
//
//	2025+ : class="… dc-icon--gem dc-icon-layout--top …"
//	rev2021: class="mwbHeadingIcon and treasures--rev2021"
//
// Everything else about those headings — the words, the script, the reading
// direction — changes with the language. This does not.
func iconMarker(n *html.Node) string {
	for _, c := range strings.Fields(attr(n, "class")) {
		if rest, ok := strings.CutPrefix(c, "dc-icon--"); ok && rest != "" {
			return rest
		}
		if rest, ok := strings.CutSuffix(c, "--rev2021"); ok && rest != "" {
			return rest
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func (p *parser) walk(n *html.Node, st style) {
	switch n.Type {
	case html.TextNode:
		if p.cur != nil {
			p.cur.Runs = append(p.cur.Runs, Run{Text: n.Data, Bold: st.bold, Italic: st.italic, Link: st.link})
		}
		return
	case html.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.walk(c, st)
		}
		return
	}

	switch n.DataAtom {
	case atom.Label, atom.Textarea, atom.Rt, atom.Rp, atom.Script, atom.Style:
		return
	case atom.Br:
		if p.cur != nil {
			p.cur.Runs = append(p.cur.Runs, Run{Text: "\n", Link: st.link})
		}
		return
	case atom.Hr:
		return
	}
	switch {
	case hasClass(n, "gen-field"):
		// Answer box: whatever came right before it is a prompt.
		if p.last != nil {
			p.last.Answer = true
		}
		return
	case hasClass(n, "pageNum"), hasClass(n, "tt"):
		return
	case hasClass(n, "parNum"):
		if p.cur != nil {
			if v, err := strconv.Atoi(attr(n, "data-pnum")); err == nil {
				p.cur.Num = v
			}
		}
		return
	case n.DataAtom == atom.Span && hasClass(n, "fn") && attr(n, "data-fnid") != "":
		if p.cur != nil {
			p.cur.Runs = append(p.cur.Runs, Run{FnMark: firstText(n)})
		}
		return
	case n.DataAtom == atom.A && hasClass(n, "fn-symbol"):
		if p.cur != nil {
			p.cur.FnLabel = strings.TrimSpace(textOf(n))
		}
		return
	}

	switch n.DataAtom {
	case atom.Figure:
		p.figure = &Image{PID: p.lastPID()}
		p.doc.Images = append(p.doc.Images, p.figure)
		p.doc.Items = append(p.doc.Items, Item{Image: p.figure})
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.walk(c, st)
		}
		p.figure = nil
		return
	case atom.Img:
		if p.figure != nil && p.figure.File == "" {
			p.figure.File = strings.TrimPrefix(attr(n, "src"), "jwpub-media://")
			p.figure.Alt = attr(n, "alt")
			p.figure.Width, _ = strconv.Atoi(attr(n, "width"))
			p.figure.Height, _ = strconv.Atoi(attr(n, "height"))
			if st.link != nil && st.link.Video != nil {
				p.figure.Video = st.link.Video
			}
		}
		return
	case atom.Video:
		if v := parseVideo(attr(n, "data-video"), ""); v != nil {
			p.addVideo(*v)
		}
		return
	case atom.B, atom.Strong:
		st.bold = true
	case atom.I, atom.Em:
		st.italic = true
	case atom.Aside:
		st.inBox = true
	case atom.Li:
		st.inItem = true
	case atom.Figcaption:
		st.inCaption = true
	case atom.A:
		l := parseLink(attr(n, "href"), attr(n, "data-video"), textOf(n))
		st.link = l
		if p.cur != nil {
			p.cur.Links = append(p.cur.Links, l)
		}
		if l.Video != nil {
			p.addVideo(*l.Video)
		}
	}
	if hasClass(n, "boxSupplement") || hasClass(n, "blockTeach") || hasClass(n, "boxContent") {
		st.inBox = true
	}
	if m := iconMarker(n); m != "" {
		st.marker = m
	}
	if hasClass(n, "fn-ref") || hasClass(n, "groupFootnote") {
		st.inFootnote = true
	}

	pidStr := attr(n, "data-pid")
	if pidStr == "" || p.cur != nil {
		if p.cur != nil && p.cur.Kind == KindOther {
			if lvl := headingLevel(n); lvl > 0 {
				p.cur.Kind, p.cur.Level = KindHeading, lvl
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.walk(c, st)
		}
		return
	}

	pid, _ := strconv.Atoi(pidStr)
	b := &Block{PID: pid, Kind: blockKind(n, st), InBox: st.inBox, Marker: st.marker}
	// A glossary entry puts its id on the list item, so the term and its
	// definition would otherwise become one block, with the whole definition
	// swallowed by the heading. The classes that mark them (de, dt, dd) are
	// structural and untranslated — verified in Spanish and Japanese — so the
	// two can be told apart without reading either.
	if term := definitionTerm(n); term != "" {
		b.Term, b.definition = term, definitionBody(n)
		b.Kind = KindDefinition
	}
	if cls := strings.Fields(attr(n, "class")); len(cls) > 0 {
		b.Class = cls[0]
		b.Classes = cls
	}
	if m := iconMarker(n); m != "" {
		b.Marker = m
	}
	if b.Kind == KindHeading {
		b.Level = headingLevel(n)
	}
	if rel := strings.Trim(attr(n, "data-rel-pid"), "[]"); rel != "" {
		first, _, _ := strings.Cut(rel, ",")
		b.RelPID, _ = strconv.Atoi(strings.TrimSpace(first))
	}
	p.cur = b
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.walk(c, st)
	}
	p.cur = nil
	p.doc.Blocks = append(p.doc.Blocks, b)
	p.doc.Items = append(p.doc.Items, Item{Block: b})
	p.last = b
	if p.figure != nil {
		switch b.Kind {
		case KindCaption:
			p.figure.Caption = joinText(p.figure.Caption, b.Text())
		case KindCredit:
			p.figure.Credit = joinText(p.figure.Credit, b.Text())
		}
	}
}

func joinText(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

func (p *parser) lastPID() int {
	if p.last != nil {
		return p.last.PID
	}
	return 0
}

func (p *parser) addVideo(v VideoRef) {
	for i := range p.doc.Videos {
		if p.doc.Videos[i].Key == v.Key {
			if p.doc.Videos[i].Title == "" {
				p.doc.Videos[i].Title = v.Title
			}
			if p.cur != nil {
				p.cur.addVideo(v)
			}
			return
		}
	}
	p.doc.Videos = append(p.doc.Videos, v)
	if p.cur != nil {
		p.cur.addVideo(v)
	}
}

func (b *Block) addVideo(v VideoRef) {
	for _, have := range b.Videos {
		if have.Key == v.Key {
			return
		}
	}
	b.Videos = append(b.Videos, v)
}

func headingLevel(n *html.Node) int {
	switch n.DataAtom {
	case atom.H1:
		return 1
	case atom.H2:
		return 2
	case atom.H3:
		return 3
	case atom.H4, atom.H5, atom.H6:
		return 4
	}
	return 0
}

// definitionTerm returns the word a definition-list entry defines, or "" when
// this element is not one.
func definitionTerm(n *html.Node) string {
	if !hasClass(n, "de") {
		return ""
	}
	var term string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if term != "" {
			return
		}
		if hasClass(x, "dt") {
			term = strings.TrimSpace(textOf(x))
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return term
}

// definitionBody returns the text of a definition-list entry without its term.
func definitionBody(n *html.Node) string {
	var body string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if body != "" {
			return
		}
		if hasClass(x, "dd") {
			body = strings.TrimSpace(textOf(x))
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return body
}

func blockKind(n *html.Node, st style) string {
	switch {
	case headingLevel(n) > 0:
		return KindHeading
	case st.inFootnote:
		return KindFootnote
	case hasClass(n, "imgCredit"):
		return KindCredit
	case st.inCaption:
		return KindCaption
	case hasClass(n, "qu"):
		return KindQuestion
	case hasClass(n, "themeScrp"):
		return KindTheme
	case hasClass(n, "contextTtl"):
		return KindContext
	case hasClass(n, "pubRefs"):
		return KindMeta
	case n.DataAtom == atom.P && st.inItem:
		return KindItem
	case n.DataAtom == atom.P:
		return KindPara
	}
	return KindOther
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && (hasClass(n, "tt") || n.DataAtom == atom.Rt) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return collapse(b.String())
}

func firstText(n *html.Node) string {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			return strings.TrimSpace(c.Data)
		}
	}
	return ""
}

var parsRe = regexp.MustCompile(`^(\d+)(?::\d+)?(?:-(\d+)(?::\d+)?)?`)

// parseLink classifies an anchor: Bible (jwpub://b/...), publication
// (jwpub://p/S:docid/pars), video (data-video or finder?lank=) or web.
func parseLink(href, dataVideo, text string) *Link {
	l := &Link{Href: href, Text: text}
	if v := parseVideo(dataVideo, href); v != nil {
		l.Kind, l.Video = "video", v
		v.Title = text
		return l
	}
	switch {
	case strings.HasPrefix(href, "jwpub://b/"):
		if r, ok := bible.ParseLink(href); ok {
			l.Kind, l.Bible = "bible", &r
			return l
		}
	case strings.HasPrefix(href, "jwpub://p/"):
		// jwpub://p/S:2013043/22-22 or jwpub://p/S:1102016902/
		rest := strings.TrimPrefix(href, "jwpub://p/")
		_, rest, _ = strings.Cut(rest, ":")
		idStr, pars, _ := strings.Cut(rest, "/")
		l.Kind = "pub"
		l.DocID, _ = strconv.Atoi(idStr)
		l.Pars = pars
		if m := parsRe.FindStringSubmatch(pars); m != nil {
			l.First, _ = strconv.Atoi(m[1])
			l.Last = l.First
			if m[2] != "" {
				l.Last, _ = strconv.Atoi(m[2])
			}
		}
		return l
	case strings.HasPrefix(href, "#"):
		l.Kind = "interno"
		return l
	}
	l.Kind = "web"
	return l
}

// parseVideo understands webpubvid://?pub=jwb-125&track=4 and jw.org finder
// links (?lank=pub-jwb-125_4_VIDEO), and builds the mediator key.
func parseVideo(dataVideo, href string) *VideoRef {
	if strings.HasPrefix(dataVideo, "webpubvid://") {
		q, err := url.ParseQuery(strings.TrimPrefix(strings.TrimPrefix(dataVideo, "webpubvid://"), "?"))
		if err == nil {
			v := &VideoRef{Pub: q.Get("pub"), Issue: q.Get("issue")}
			v.Track, _ = strconv.Atoi(q.Get("track"))
			v.DocID, _ = strconv.Atoi(q.Get("docid"))
			v.Key = VideoKey(v.Pub, v.Issue, v.Track, v.DocID)
			if v.Key != "" {
				return v
			}
		}
	}
	if u, err := url.Parse(href); err == nil && strings.Contains(u.Host, "jw.org") {
		if lank := u.Query().Get("lank"); lank != "" {
			return &VideoRef{Key: lank}
		}
	}
	return nil
}

// VideoKey builds the language-agnostic natural key the mediator API uses.
func VideoKey(pub, issue string, track, docid int) string {
	switch {
	case docid != 0:
		return fmt.Sprintf("docid-%d_%d_VIDEO", docid, max(track, 1))
	case pub == "":
		return ""
	case issue != "" && issue != "0":
		return fmt.Sprintf("pub-%s_%s_%d_VIDEO", pub, strings.TrimSuffix(issue, "00"), track)
	default:
		return fmt.Sprintf("pub-%s_%d_VIDEO", pub, track)
	}
}

var subentryRe = regexp.MustCompile(`^\d+\.$`)

// numberSubentries numbers the paragraphs of Insight articles split into
// "1.", "2."... senses. Publications cite them per sense, and the reader app
// leaves such paragraphs without
// a parNum, so the count restarts at every bold "N." that opens a paragraph.
func (d *Doc) numberSubentries() {
	sub, count := 0, 0
	has := false
	for _, b := range d.Blocks {
		if b.Kind == KindPara && b.Class == "sb" && b.leadingBoldNumber() > 0 {
			has = true
			break
		}
	}
	if !has {
		return
	}
	for _, b := range d.Blocks {
		if b.Class == "sn" {
			b.Num = 0 // the etymology line carries a parNum that nobody cites
		}
	}
	for _, b := range d.Blocks {
		if b.Kind != KindPara || b.Class != "sb" {
			continue
		}
		if n := b.leadingBoldNumber(); n > 0 {
			sub, count = n, 0
		}
		if sub == 0 || b.Num != 0 {
			continue
		}
		count++
		b.Sub, b.Num = sub, count
	}
}

func (b *Block) leadingBoldNumber() int {
	for _, r := range b.Runs {
		t := strings.TrimSpace(r.Text)
		if t == "" && r.FnMark == "" {
			continue
		}
		if r.Bold && subentryRe.MatchString(t) {
			n, _ := strconv.Atoi(strings.TrimSuffix(t, "."))
			return n
		}
		return 0
	}
	return 0
}

var qNumRe = regexp.MustCompile(`^[\s\x{a0}]*(\d+)((?:[\s\x{a0}]*,[\s\x{a0}]*\d+)*)[\s\x{a0}]*\.[\s\x{a0}]*`)

// finish derives question numbers from the leading "12." of study questions.
func (b *Block) finish() {
	if b.Kind != KindQuestion {
		return
	}
	text := b.Text()
	m := qNumRe.FindStringSubmatch(text)
	if m == nil {
		return
	}
	b.Num, _ = strconv.Atoi(m[1])
	b.NumLabel = strings.Join(strings.FieldsFunc(m[1]+m[2], func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}), ", ")
	b.trimLeading(len([]rune(strings.Join(strings.Fields(m[0]), ""))))
}

// trimLeading drops the first n non-space characters of the visible text
// (the "12." of a question) and the whitespace that follows them.
func (b *Block) trimLeading(n int) {
	for i := range b.Runs {
		r := &b.Runs[i]
		if r.FnMark != "" {
			break
		}
		var kept []rune
		for _, ch := range r.Text {
			if n > 0 {
				if !unicode.IsSpace(ch) {
					n--
				}
				continue
			}
			kept = append(kept, ch)
		}
		r.Text = string(kept)
		if n == 0 {
			r.Text = strings.TrimLeft(r.Text, " \t\r\n\u00a0")
			if r.Text != "" {
				break
			}
		}
	}
	for len(b.Runs) > 0 && b.Runs[0].Text == "" && b.Runs[0].FnMark == "" {
		b.Runs = b.Runs[1:]
	}
}

var spaceRe = regexp.MustCompile(`[ \t\r\n]+`)

// collapse squeezes ASCII whitespace but keeps no-break spaces, which are part
// of the published text.
func collapse(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// Text is the plain text of the block, without footnote calls.
func (b *Block) Text() string {
	// A glossary entry's text is its definition. The term is in Term, and
	// repeating it here would put the word twice in every rendering.
	if b.Kind == KindDefinition && b.definition != "" {
		return collapse(b.definition)
	}
	var sb strings.Builder
	for _, r := range b.Runs {
		sb.WriteString(r.Text)
	}
	return collapse(sb.String())
}

// BibleRefs lists the Bible links of the block.
func (b *Block) BibleRefs() []bible.Range {
	var out []bible.Range
	for _, l := range b.Links {
		if l.Kind == "bible" && l.Bible != nil {
			out = append(out, *l.Bible)
		}
	}
	return out
}

// PubLinks lists the links to other publications.
func (b *Block) PubLinks() []*Link {
	var out []*Link
	for _, l := range b.Links {
		if l.Kind == "pub" {
			out = append(out, l)
		}
	}
	return out
}

// IsQuestion reports whether the block asks something the reader answers:
// a study question, or a prompt followed by an answer box.
func (b *Block) IsQuestion() bool {
	return b.Kind == KindQuestion || (b.Answer && b.Kind != KindHeading)
}
