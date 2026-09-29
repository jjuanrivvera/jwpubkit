package bible

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Range is a contiguous run of verses inside one book.
type Range struct {
	Book         int `json:"book"`
	StartChapter int `json:"start_chapter"`
	StartVerse   int `json:"start_verse"`
	EndChapter   int `json:"end_chapter"`
	EndVerse     int `json:"end_verse"`
}

// FirstID and LastID bound the range in BibleVerseId numbering.
func (r Range) FirstID() int {
	id, _ := VerseID(r.Book, r.StartChapter, r.StartVerse)
	return id
}

func (r Range) LastID() int {
	id, _ := VerseID(r.Book, r.EndChapter, r.EndVerse)
	return id
}

// Contains reports whether the verse id falls inside the range.
func (r Range) Contains(id int) bool { return id >= r.FirstID() && id <= r.LastID() }

func (r Range) wholeChapters() bool {
	return r.StartVerse <= 1 && r.EndVerse == VerseCount(r.Book, r.EndChapter)
}

func (r Range) format(book string) string {
	switch {
	case ChapterCount(r.Book) == 1:
		if r.StartVerse == r.EndVerse {
			return fmt.Sprintf("%s %d", book, r.StartVerse)
		}
		return fmt.Sprintf("%s %d-%d", book, r.StartVerse, r.EndVerse)
	case r.wholeChapters() && r.StartChapter == r.EndChapter:
		return fmt.Sprintf("%s %d", book, r.StartChapter)
	case r.wholeChapters():
		return fmt.Sprintf("%s %d-%d", book, r.StartChapter, r.EndChapter)
	case r.StartChapter != r.EndChapter:
		return fmt.Sprintf("%s %d:%d-%d:%d", book, r.StartChapter, r.StartVerse, r.EndChapter, r.EndVerse)
	case r.StartVerse == r.EndVerse:
		return fmt.Sprintf("%s %d:%d", book, r.StartChapter, r.StartVerse)
	default:
		return fmt.Sprintf("%s %d:%d-%d", book, r.StartChapter, r.StartVerse, r.EndVerse)
	}
}

// String uses the short abbreviation: "Jer 38:1-13".
func (r Range) String() string {
	b, _ := BookByNum(r.Book)
	return r.format(b.Short)
}

// Long uses the full book name: "Jeremiah 38:1-13".
func (r Range) Long() string {
	b, _ := BookByNum(r.Book)
	return r.format(b.Name)
}

// Verses enumerates the (chapter, verse) pairs of the range.
func (r Range) Verses() [][2]int {
	var out [][2]int
	for c := r.StartChapter; c <= r.EndChapter; c++ {
		from, to := 1, VerseCount(r.Book, c)
		if c == r.StartChapter {
			from = r.StartVerse
		}
		if c == r.EndChapter {
			to = r.EndVerse
		}
		for v := from; v <= to; v++ {
			out = append(out, [2]int{c, v})
		}
	}
	return out
}

// FromIDs builds a range from two BibleVerseIds of the same book.
func FromIDs(first, last int) (Range, bool) {
	b1, c1, v1, ok1 := Locate(first)
	b2, c2, v2, ok2 := Locate(last)
	if !ok1 || !ok2 || b1 != b2 {
		return Range{}, false
	}
	return Range{b1, c1, v1, c2, v2}, true
}

var linkRe = regexp.MustCompile(`^jwpub://b/[A-Za-z0-9]+/(\d+):(\d+):(\d+)(?:-(\d+):(\d+):(\d+))?`)

// ParseLink reads the Bible links JWPUB documents use, e.g.
// "jwpub://b/NWTR/24:38:1-24:38:13".
func ParseLink(href string) (Range, bool) {
	m := linkRe.FindStringSubmatch(href)
	if m == nil {
		return Range{}, false
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	r := Range{Book: n(m[1]), StartChapter: n(m[2]), StartVerse: n(m[3])}
	r.EndChapter, r.EndVerse = r.StartChapter, r.StartVerse
	if m[4] != "" && n(m[4]) == r.Book {
		r.EndChapter, r.EndVerse = n(m[5]), n(m[6])
	}
	if r.Book < 1 || r.Book > 66 {
		return Range{}, false
	}
	return r, true
}

// bookPrefix splits "1 Cor. 13:4" into "1 Cor." and "13:4". The book part is
// an optional 1-3, then letters (spaces and dots allowed) up to the first
// digit.
var bookPrefix = regexp.MustCompile(`^\s*([1-3]\s*)?([\p{L}][\p{L}\s.]*?)\.?\s*(\d.*)?$`)

// Parse reads one or more references separated by ";":
//
//	"Jer 38:6", "Jer 38:1-13", "Jeremiah 38, 39", "1 Cor. 13:4-7",
//	"Sal 23", "Jer 38:28–39:2", "Jer 38:6; 39:1, 4-6", "3 Juan 3, 4".
//
// A segment without a book name continues the previous book.
func Parse(s string) ([]Range, error) {
	s = strings.NewReplacer("–", "-", "—", "-", " ", " ", "‑", "-").Replace(s)
	var out []Range
	book := 0
	for _, seg := range strings.Split(s, ";") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		rest := seg
		if m := bookPrefix.FindStringSubmatch(seg); m != nil {
			name := strings.TrimSpace(m[1] + m[2])
			b, ok := LookupBook(name)
			if !ok {
				return nil, fmt.Errorf("unknown book: %q", name)
			}
			book = b.Num
			rest = m[3]
		} else if book == 0 {
			return nil, fmt.Errorf("falta el libro en %q", seg)
		}
		rs, err := parseNumbers(book, strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("%q: %w", seg, err)
		}
		out = append(out, rs...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no hay ninguna referencia en %q", s)
	}
	return out, nil
}

// parseNumbers handles the numeric part: chapter lists ("38, 39", "38-40")
// when there is no colon, verse lists otherwise.
func parseNumbers(book int, s string) ([]Range, error) {
	oneChapter := ChapterCount(book) == 1
	if s == "" {
		if oneChapter {
			return []Range{{book, 1, 1, 1, VerseCount(book, 1)}}, nil
		}
		return nil, fmt.Errorf("the chapter is missing")
	}
	var out []Range
	chapter := 0
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		a, b, _ := strings.Cut(item, "-")
		c1, v1, err := splitCV(a)
		if err != nil {
			return nil, err
		}
		hasColon := strings.Contains(s, ":")
		var r Range
		switch {
		case oneChapter && c1 == 0:
			r = Range{book, 1, v1, 1, v1}
		case oneChapter:
			r = Range{book, 1, v1, 1, v1}
		case c1 == 0 && !hasColon:
			// Whole chapter: "Jer 38". A Psalm heading belongs to it.
			r = Range{book, v1, firstVerse(book, v1), v1, VerseCount(book, v1)}
		case c1 == 0:
			if chapter == 0 {
				return nil, fmt.Errorf("verse %d has no chapter", v1)
			}
			r = Range{book, chapter, v1, chapter, v1}
		default:
			chapter = c1
			r = Range{book, c1, v1, c1, v1}
		}
		if b != "" {
			c2, v2, err := splitCV(b)
			if err != nil {
				return nil, err
			}
			switch {
			case c2 != 0:
				r.EndChapter, r.EndVerse = c2, v2
				chapter = c2
			case !oneChapter && !hasColon:
				r.EndChapter, r.EndVerse = v2, VerseCount(book, v2)
			default:
				r.EndVerse = v2
			}
		}
		if err := validate(r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func firstVerse(book, chapter int) int {
	if HasSuperscription(book, chapter) {
		return 0
	}
	return 1
}

// splitCV reads "38:6" as (38, 6) and "6" as (0, 6).
func splitCV(s string) (int, int, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
	c, v, found := strings.Cut(s, ":")
	if !found {
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid number %q", s)
		}
		return 0, n, nil
	}
	cn, err1 := strconv.Atoi(strings.TrimSpace(c))
	vn, err2 := strconv.Atoi(strings.TrimSpace(v))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid reference %q", s)
	}
	return cn, vn, nil
}

func validate(r Range) error {
	bk, _ := BookByNum(r.Book)
	name := bk.Name
	for _, cv := range [][2]int{{r.StartChapter, r.StartVerse}, {r.EndChapter, r.EndVerse}} {
		if cv[0] < 1 || cv[0] > ChapterCount(r.Book) {
			return fmt.Errorf("%s has no chapter %d", name, cv[0])
		}
		if _, ok := VerseID(r.Book, cv[0], cv[1]); !ok {
			return fmt.Errorf("%s %d has no verse %d", name, cv[0], cv[1])
		}
	}
	if r.EndChapter < r.StartChapter || (r.EndChapter == r.StartChapter && r.EndVerse < r.StartVerse) {
		return fmt.Errorf("the range runs backwards in %s", r.Long())
	}
	return nil
}

// FormatList renders several ranges compactly, repeating the book only when
// it changes: "Jer 38:7-9; 39:15-18".
func FormatList(rs []Range) string {
	var parts []string
	prev := 0
	for _, r := range rs {
		s := r.String()
		if r.Book == prev {
			b, _ := BookByNum(r.Book)
			s = strings.TrimPrefix(s, b.Short+" ")
		}
		parts = append(parts, s)
		prev = r.Book
	}
	return strings.Join(parts, "; ")
}
