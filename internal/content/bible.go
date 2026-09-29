package content

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// InnerText strips the markup of a fragment, skipping tooltip placeholders,
// and collapses whitespace (no-break spaces are kept).
func InnerText(src string) string {
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return collapse(src)
	}
	var sb strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			sb.WriteString(n.Data)
			return
		case html.ElementNode:
			if hasClass(n, "tt") || n.DataAtom == atom.Rt || n.DataAtom == atom.Label || n.DataAtom == atom.Textarea {
				return
			}
			if n.DataAtom == atom.Br || n.DataAtom == atom.P || n.DataAtom == atom.Div || n.DataAtom == atom.Li {
				defer sb.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	for _, n := range nodes {
		rec(n)
	}
	return collapse(sb.String())
}

// VerseText reads a BibleVerse.Content fragment,
// `<span id="v24-38-6" class="v">…</span>`, into book, chapter, verse and text.
func VerseText(src string) (book, chapter, verse int, text string, ok bool) {
	i := strings.Index(src, `id="v`)
	if i < 0 {
		return 0, 0, 0, "", false
	}
	rest := src[i+len(`id="v`):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return 0, 0, 0, "", false
	}
	parts := strings.Split(rest[:end], "-")
	if len(parts) < 3 {
		return 0, 0, 0, "", false
	}
	book, _ = strconv.Atoi(parts[0])
	chapter, _ = strconv.Atoi(parts[1])
	verse, _ = strconv.Atoi(parts[2])
	return book, chapter, verse, InnerText(src), true
}

// Mark locates a footnote call or a marginal-reference letter in the Bible
// text: the verse it belongs to, its letter and the word it follows.
type Mark struct {
	Book, Chapter, Verse int
	Letter               string
	Anchor               string
}

// ChapterMarks walks BibleChapter.Content and indexes footnote calls by
// data-fnid (Footnote.FootnoteIndex) and marginal letters by data-mid
// (BibleCitation.BlockNumber).
func ChapterMarks(src string) (fns, mids map[int]Mark) {
	fns, mids = map[int]Mark{}, map[int]Mark{}
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return fns, mids
	}
	var b, c, v int
	var buf strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
			return
		}
		if n.Type != html.ElementNode {
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				rec(ch)
			}
			return
		}
		switch {
		case hasClass(n, "tt"), hasClass(n, "vl"), hasClass(n, "cl"):
			return
		case n.DataAtom == atom.Span && hasClass(n, "v"):
			if id := attr(n, "id"); strings.HasPrefix(id, "v") {
				parts := strings.Split(id[1:], "-")
				if len(parts) >= 3 {
					nb, _ := strconv.Atoi(parts[0])
					nc, _ := strconv.Atoi(parts[1])
					nv, _ := strconv.Atoi(parts[2])
					if nb != b || nc != c || nv != v {
						buf.Reset()
					}
					b, c, v = nb, nc, nv
				}
			}
		case hasClass(n, "fn") && attr(n, "data-fnid") != "":
			id, _ := strconv.Atoi(attr(n, "data-fnid"))
			fns[id] = Mark{b, c, v, firstText(n), lastWord(buf.String())}
			return
		case hasClass(n, "m") && attr(n, "data-mid") != "":
			id, _ := strconv.Atoi(attr(n, "data-mid"))
			mids[id] = Mark{b, c, v, firstText(n), lastWord(buf.String())}
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			rec(ch)
		}
	}
	for _, n := range nodes {
		rec(n)
	}
	return fns, mids
}

func lastWord(s string) string {
	s = strings.TrimRightFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	i := strings.LastIndexFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' })
	if i < 0 {
		return s
	}
	_, w := utf8.DecodeRuneInString(s[i:])
	return s[i+w:]
}
