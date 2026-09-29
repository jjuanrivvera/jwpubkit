package bible

import (
	"strconv"
	"strings"
)

type chapterInfo struct {
	verses  int  // numbered verses, superscription excluded
	sup     bool // Psalm superscription stored as verse 0
	firstID int  // BibleVerseId of verse 1 (or of the superscription)
}

var chapters [67][]chapterInfo // chapters[book][chapter-1]

// TotalVerseIDs is the number of BibleVerseId values (0-based, contiguous).
var TotalVerseIDs int

func init() {
	id := 0
	for b, spec := range versification {
		for _, item := range strings.Split(spec, ",") {
			ci := chapterInfo{firstID: id}
			if strings.HasSuffix(item, "s") {
				ci.sup = true
				item = strings.TrimSuffix(item, "s")
			}
			n, err := strconv.Atoi(item)
			if err != nil {
				panic("bible: versificación corrupta: " + spec)
			}
			ci.verses = n
			chapters[b+1] = append(chapters[b+1], ci)
			id += n
			if ci.sup {
				id++
			}
		}
	}
	TotalVerseIDs = id
}

// ChapterCount returns how many chapters book has.
func ChapterCount(book int) int {
	if book < 1 || book > 66 {
		return 0
	}
	return len(chapters[book])
}

// VerseCount returns the number of verses of a chapter (0 if it does not exist).
func VerseCount(book, chapter int) int {
	if chapter < 1 || chapter > ChapterCount(book) {
		return 0
	}
	return chapters[book][chapter-1].verses
}

// HasSuperscription reports whether a Psalm has a heading stored as verse 0.
func HasSuperscription(book, chapter int) bool {
	if chapter < 1 || chapter > ChapterCount(book) {
		return false
	}
	return chapters[book][chapter-1].sup
}

// VerseID converts book/chapter/verse to the BibleVerseId used by BibleVerse,
// BibleCitation and VerseCommentaryMap. Verse 0 is a Psalm superscription.
func VerseID(book, chapter, verse int) (int, bool) {
	if chapter < 1 || chapter > ChapterCount(book) {
		return 0, false
	}
	ci := chapters[book][chapter-1]
	switch {
	case verse == 0 && ci.sup:
		return ci.firstID, true
	case verse < 1 || verse > ci.verses:
		return 0, false
	case ci.sup:
		return ci.firstID + verse, true
	default:
		return ci.firstID + verse - 1, true
	}
}

// Locate is the inverse of VerseID.
func Locate(id int) (book, chapter, verse int, ok bool) {
	if id < 0 || id >= TotalVerseIDs {
		return 0, 0, 0, false
	}
	for b := 1; b <= 66; b++ {
		chs := chapters[b]
		last := chs[len(chs)-1]
		end := last.firstID + last.verses
		if last.sup {
			end++
		}
		if id >= end {
			continue
		}
		for c := len(chs) - 1; c >= 0; c-- {
			ci := chs[c]
			if id < ci.firstID {
				continue
			}
			v := id - ci.firstID + 1
			if ci.sup {
				v--
			}
			return b, c + 1, v, true
		}
	}
	return 0, 0, 0, false
}
