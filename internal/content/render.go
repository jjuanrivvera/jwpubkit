package content

import (
	"fmt"
	"strings"
)

// WolDocURL is the wol.jw.org address of a document, optionally at a paragraph.
func WolDocURL(docid, pid int) string {
	return DocURL(docid, pid)
}

// language is the jw.org symbol used when addressing published content. It is
// set once at startup; the zero value keeps the addresses working by letting
// jw.org choose, rather than silently sending everyone to one language.
var language = ""

// UseLanguage sets the language every address this package builds points at.
func UseLanguage(lang string) { language = lang }

// DocURL addresses a document, and a paragraph within it, in the language in
// use.
//
// It uses jw.org's finder with the MEPS document id rather than a wol.jw.org
// path, because a wol path embeds a per-language repository id and library code
// (/es/wol/d/r4/lp-s/…) that cannot be derived from anything in a JWPUB. The
// finder takes the docid and the language symbol we already have, resolves to
// the right article in that language, and honors a paragraph: verified in both
// Spanish and English, landing on the #p anchor.
func DocURL(docid, pid int) string {
	u := fmt.Sprintf("https://www.jw.org/finder?docid=%d", docid)
	if pid > 0 {
		u += fmt.Sprintf("&par=%d", pid)
	}
	if language != "" {
		u += "&wtlocale=" + language
	}
	return u
}

// FinderURL is the jw.org page that plays a video.
func FinderURL(key string) string {
	u := "https://www.jw.org/finder?lank=" + key
	if language != "" {
		u += "&wtlocale=" + language
	}
	return u
}

// RenderOptions tunes Markdown output.
type RenderOptions struct {
	DocID    int                     // links to this document are rendered as plain text
	ImageRef func(img *Image) string // where an image is shown from; defaults to its file name
}

// Markdown renders the document as clean Markdown: headings, numbered
// paragraphs, study questions as quotes, images with their captions,
// publication references as wol links and footnotes in GFM syntax.
func (d *Doc) Markdown(opt RenderOptions) string {
	var out []string
	for _, it := range d.Items {
		if it.Image != nil {
			out = append(out, imageMarkdown(it.Image, opt))
			continue
		}
		b := it.Block
		plain := b.Kind == KindHeading || b.Kind == KindContext
		body := b.markdownRuns(opt, plain)
		if strings.TrimSpace(body) == "" {
			continue
		}
		var line string
		switch b.Kind {
		case KindCaption, KindCredit:
			continue // printed with the image
		case KindHeading:
			line = strings.Repeat("#", min(max(b.Level, 1), 4)) + " " + body
		case KindContext:
			line = "*" + body + "*"
		case KindTheme:
			line = "> " + body
		case KindQuestion:
			label := b.NumLabel
			if label == "" && b.Num > 0 {
				label = fmt.Sprint(b.Num)
			}
			if label != "" {
				line = fmt.Sprintf("> **%s.** %s", label, body)
			} else {
				line = "> " + body
			}
		case KindItem:
			line = "- " + body
		case KindFootnote:
			if b.FnLabel != "" {
				line = fmt.Sprintf("[^%s]: %s", b.FnLabel, body)
			} else {
				line = body
			}
		case KindPara:
			switch {
			case b.Sub > 0:
				line = fmt.Sprintf("**[sense %d · par. %d]** %s", b.Sub, b.Num, body)
			case b.Num > 0:
				line = fmt.Sprintf("**%d** %s", b.Num, body)
			default:
				line = body
			}
		default:
			line = body
		}
		if b.InBox && b.Kind != KindFootnote {
			line = "> " + strings.ReplaceAll(line, "\n", "\n> ")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n\n") + "\n"
}

func imageMarkdown(img *Image, opt RenderOptions) string {
	ref := img.File
	if opt.ImageRef != nil {
		ref = opt.ImageRef(img)
	}
	alt := strings.ReplaceAll(img.Alt, "]", ")")
	alt = strings.ReplaceAll(alt, "[", "(")
	s := fmt.Sprintf("![%s](%s)", alt, ref)
	if img.Caption != "" {
		s += "\n*" + escapeMD(img.Caption) + "*"
	}
	if img.Credit != "" {
		s += "\n<small>" + img.Credit + "</small>"
	}
	if img.Video != nil {
		s += fmt.Sprintf("\n(video: `%s`)", img.Video.Key)
	}
	return s
}

func escapeMD(s string) string {
	return strings.NewReplacer("*", `\*`, "_", `\_`).Replace(s)
}

// segment is a run of runs sharing link and style.
type segment struct {
	text         string
	bold, italic bool
	link         *Link
	fn           string
}

// segments merges runs that render alike. Links without a target (Bible
// references, same-document links) count as plain text so that "(lee Juan
// 8:29)." stays one bold span.
func (b *Block) segments(opt RenderOptions, plain bool) []segment {
	var segs []segment
	for _, r := range b.Runs {
		if plain {
			r.Bold, r.Italic = false, false
		}
		link := r.Link
		if link != nil && linkTarget(link, opt) == "" {
			link = nil
		}
		if r.FnMark != "" {
			segs = append(segs, segment{fn: r.FnMark, link: link})
			continue
		}
		t := spaceRe.ReplaceAllString(r.Text, " ")
		n := len(segs)
		if n > 0 && segs[n-1].fn == "" && strings.TrimSpace(t) == "" && segs[n-1].link == link {
			segs[n-1].text += t // a bare space takes the style around it
			continue
		}
		if n > 0 && segs[n-1].fn == "" && segs[n-1].link == link &&
			segs[n-1].bold == r.Bold && segs[n-1].italic == r.Italic {
			segs[n-1].text += t
			continue
		}
		segs = append(segs, segment{text: t, bold: r.Bold, italic: r.Italic, link: link})
	}
	return segs
}

func emphasize(s segment) string {
	t := escapeMD(s.text)
	if strings.TrimSpace(t) == "" || (!s.bold && !s.italic) {
		return t
	}
	lead := t[:len(t)-len(strings.TrimLeft(t, " "))]
	trail := t[len(strings.TrimRight(t, " ")):]
	core := strings.TrimSpace(t)
	mark := "*"
	switch {
	case s.bold && s.italic:
		mark = "***"
	case s.bold:
		mark = "**"
	}
	return lead + mark + core + mark + trail
}

// markdownRuns renders the runs; plain drops bold and italics (headings are
// already emphasized by their level).
func (b *Block) markdownRuns(opt RenderOptions, plain bool) string {
	var sb strings.Builder
	segs := b.segments(opt, plain)
	for i := 0; i < len(segs); {
		s := segs[i]
		if s.fn != "" {
			fmt.Fprintf(&sb, "[^%s]", s.fn)
			i++
			continue
		}
		if s.link == nil {
			sb.WriteString(emphasize(s))
			i++
			continue
		}
		// A link can span several styled segments ("<em>w13</em> 15/1 9").
		j := i
		var inner strings.Builder
		for j < len(segs) && segs[j].link == s.link && segs[j].fn == "" {
			inner.WriteString(emphasize(segs[j]))
			j++
		}
		text := inner.String()
		lead := text[:len(text)-len(strings.TrimLeft(text, " "))]
		trail := text[len(strings.TrimRight(text, " ")):]
		core := strings.TrimSpace(text)
		if href := linkTarget(s.link, opt); href != "" && core != "" {
			fmt.Fprintf(&sb, "%s[%s](%s)%s", lead, core, href, trail)
		} else {
			sb.WriteString(text)
		}
		i = j
	}
	return strings.TrimSpace(sb.String())
}

func linkTarget(l *Link, opt RenderOptions) string {
	switch l.Kind {
	case "pub":
		if l.DocID == 0 || l.DocID == opt.DocID {
			return ""
		}
		return WolDocURL(l.DocID, l.First)
	case "video":
		if l.Video != nil {
			return FinderURL(l.Video.Key)
		}
	case "web":
		if strings.HasPrefix(l.Href, "http") {
			return l.Href
		}
	}
	return ""
}

// PlainText renders the document with flat markers — [H1] [H2] [H3]
// [QUESTION n] [IMAGE ...] [BOX] — for consumers that want the structure
// without parsing Markdown.
func (d *Doc) PlainText() string {
	var out []string
	for _, it := range d.Items {
		if img := it.Image; img != nil {
			parts := []string{"file=" + img.File}
			if img.Alt != "" {
				parts = append([]string{"alt=" + img.Alt}, parts...)
			}
			if img.Caption != "" {
				parts = append(parts, "caption="+img.Caption)
			}
			if img.Video != nil {
				parts = append(parts, "video="+img.Video.Key)
			}
			out = append(out, "[IMAGE "+strings.Join(parts, " | ")+"]")
			continue
		}
		b := it.Block
		t := b.textWithMarks()
		if t == "" {
			continue
		}
		switch b.Kind {
		case KindCaption, KindCredit:
			continue
		case KindHeading:
			t = fmt.Sprintf("[H%d] %s", max(b.Level, 1), t)
		case KindQuestion:
			label := b.NumLabel
			if label == "" && b.Num > 0 {
				label = fmt.Sprint(b.Num)
			}
			t = strings.TrimSpace(fmt.Sprintf("[QUESTION %s] %s", label, t))
			t = strings.Replace(t, "[QUESTION ] ", "[QUESTION] ", 1)
		case KindTheme:
			t = "[TEXTO TEMÁTICO] " + t
		case KindItem:
			if b.Answer {
				t = "[QUESTION] " + t
			} else {
				t = "- " + t
			}
		case KindFootnote:
			if b.FnLabel != "" {
				t = fmt.Sprintf("[NOTA %s] %s", b.FnLabel, t)
			}
		case KindPara:
			switch {
			case b.Sub > 0:
				t = fmt.Sprintf("[sense %d · par. %d] %s", b.Sub, b.Num, t)
			case b.Num > 0:
				t = fmt.Sprintf("%d %s", b.Num, t)
			}
			if b.Answer {
				t = "[QUESTION] " + t
			}
		}
		if b.InBox {
			t = "[BOX] " + t
		}
		out = append(out, t)
	}
	return strings.Join(out, "\n") + "\n"
}

// textWithMarks is Text() with footnote calls kept as "(a)".
func (b *Block) textWithMarks() string {
	var sb strings.Builder
	for _, r := range b.Runs {
		if r.FnMark != "" {
			sb.WriteString("(" + r.FnMark + ")")
			continue
		}
		sb.WriteString(r.Text)
	}
	return collapse(sb.String())
}
