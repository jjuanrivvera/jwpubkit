package content

import (
	"time"
	"unicode"
)

// ReadingRate is how fast a text is assumed to be read aloud.
//
// Two numbers, because counting words only works where words are separated. A
// script written without spaces — Chinese, Japanese, Thai — has to be counted in
// characters or the estimate is off by an order of magnitude. Which one applies
// is decided by the text itself, not by a language setting, so a quotation in
// another script inside a paragraph is still counted sensibly.
type ReadingRate struct {
	// WordsPerMinute for scripts that separate words. 130 is a deliberate,
	// unhurried reading aloud rather than a silent reading speed.
	WordsPerMinute float64
	// CharsPerMinute for scripts that do not. Han and kana are read at far fewer
	// units per minute than letters, hence a separate figure.
	CharsPerMinute float64
}

// DefaultRate is the assumption used when none is given. It is an assumption, and
// callers are told so rather than shown a figure that looks measured.
var DefaultRate = ReadingRate{WordsPerMinute: 130, CharsPerMinute: 350}

// ReadingTime estimates how long a text takes to read aloud. It counts the words
// of space-separated scripts and the characters of scripts that are not, then
// charges each at its own rate.
func ReadingTime(text string, rate ReadingRate) time.Duration {
	if rate.WordsPerMinute <= 0 {
		rate.WordsPerMinute = DefaultRate.WordsPerMinute
	}
	if rate.CharsPerMinute <= 0 {
		rate.CharsPerMinute = DefaultRate.CharsPerMinute
	}
	words, dense := CountUnits(text)
	minutes := float64(words)/rate.WordsPerMinute + float64(dense)/rate.CharsPerMinute
	return time.Duration(minutes * float64(time.Minute))
}

// CountUnits returns the number of space-separated words and, separately, the
// number of characters belonging to scripts that do not separate words. It is
// exported so a caller can report what was counted instead of only the verdict.
func CountUnits(text string) (words, dense int) {
	inWord := false
	for _, r := range text {
		switch {
		case isDenseScript(r):
			dense++
			inWord = false
		case unicode.IsSpace(r):
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				words++
				inWord = true
			}
		default:
			inWord = false
		}
	}
	return words, dense
}

// isDenseScript reports whether a rune belongs to a script written without
// spaces between words.
func isDenseScript(r rune) bool {
	switch {
	case unicode.Is(unicode.Han, r),
		unicode.Is(unicode.Hiragana, r),
		unicode.Is(unicode.Katakana, r),
		unicode.Is(unicode.Thai, r),
		unicode.Is(unicode.Lao, r),
		unicode.Is(unicode.Khmer, r),
		unicode.Is(unicode.Myanmar, r):
		return true
	}
	return false
}
