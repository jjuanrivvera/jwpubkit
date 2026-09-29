package meeting

import (
	"strings"
	"unicode"
)

// Publications render numbers in the digits of their own script: Arabic shows
// ten minutes as ١٠ and a song as ١٢٨, Chinese writes a part number with the
// fullwidth stop 1．, and several scripts have their own decimal digits
// entirely. Go's \d, strconv.Atoi and a regexp like ^(\d+)\. all quietly fail on
// every one of them, which is enough to leave a week's minutes and song numbers
// empty in most of the languages this tool is supposed to serve.
//
// Unicode guarantees that every decimal digit block is ten consecutive code
// points starting at the block's zero, so the value of a digit is its distance
// from the zero of its own block. That is derived from Go's own tables — there
// is no per-language data here.

// digitValue returns the numeric value of any Unicode decimal digit.
func digitValue(r rune) (int, bool) {
	if !unicode.IsDigit(r) {
		return 0, false
	}
	// Walk back to the zero of this digit's block.
	zero := r
	for unicode.IsDigit(zero-1) && zero-1 >= r-9 {
		zero--
	}
	return int(r - zero), true
}

// firstNumber reads the first run of decimal digits in s, in any script, and
// returns its value. It reports false when s carries no digits at all, which a
// caller must not confuse with a real zero.
func firstNumber(s string) (int, bool) {
	n, seen := 0, false
	for _, r := range s {
		v, ok := digitValue(r)
		if !ok {
			if seen {
				break
			}
			continue
		}
		n, seen = n*10+v, true
	}
	return n, seen
}

// leadingNumber reads a number that opens a string, ignoring whatever
// punctuation follows it, and returns the number with the rest of the string.
// It is how a part title is split from its number whatever punctuation, digits
// or invisible direction marks the script puts between them.
func leadingNumber(s string) (int, string, bool) {
	t := strings.TrimSpace(s)
	n, seen, i := 0, false, 0
	for _, r := range t {
		v, ok := digitValue(r)
		if !ok {
			break
		}
		n, seen = n*10+v, true
		i += len(string(r))
	}
	if !seen {
		return 0, s, false
	}
	// Only the separator between the number and the title is dropped. Trimming
	// punctuation generally would eat an opening quote and turn “A video” into
	// A video”, which is how this was wrong the first time.
	rest := strings.TrimLeftFunc(t[i:], isNumberSeparator)
	if rest == "" {
		return 0, s, false // a title that is only a number is not a numbered part
	}
	return n, rest, true
}

// isNumberSeparator reports whether r is the kind of character a publication
// puts between a part number and its title: a stop, a dash, a bracket, a colon,
// whitespace, or one of the invisible marks a right-to-left script needs.
func isNumberSeparator(r rune) bool {
	switch r {
	case '.', '\uff0e', '\u3002', '-', '\u2013', '\u2014', ')', '\uff09', ':', '\uff1a', '\u060c', ',':
		return true
	case '\u00a0', '\u200f', '\u200e', '\u202b', '\u202c':
		return true
	}
	return unicode.IsSpace(r)
}
