package content

import (
	"strings"
	"testing"
	"time"
)

// The point of the two rates: counting words in a script that has none would put
// a Chinese paragraph at a couple of seconds. Both renderings of comparable
// content should land in the same neighbourhood.
func TestReadingTimeAcrossScripts(t *testing.T) {
	latin := strings.Repeat("una palabra mas de texto inventado ", 26) // ~156 words
	got := ReadingTime(latin, DefaultRate)
	if got < time.Minute || got > 90*time.Second {
		t.Errorf("156 words read aloud = %v, want about a minute", got)
	}

	// A comparable amount of Han: about 350 characters is a minute.
	han := strings.Repeat("发明的文字内容", 50) // 350 runes
	got = ReadingTime(han, DefaultRate)
	if got < 50*time.Second || got > 80*time.Second {
		t.Errorf("350 han characters = %v, want about a minute", got)
	}

	// Counting the han text as words would have given seconds, not a minute.
	words, dense := CountUnits(han)
	if dense != 350 {
		t.Errorf("dense = %d, want 350", dense)
	}
	if words != 0 {
		t.Errorf("words = %d, want none: the script has no word breaks", words)
	}
}

func TestReadingTimeEdges(t *testing.T) {
	if got := ReadingTime("", DefaultRate); got != 0 {
		t.Errorf("empty text = %v", got)
	}
	if got := ReadingTime("   \n\t ", DefaultRate); got != 0 {
		t.Errorf("whitespace only = %v", got)
	}
	// Punctuation does not make words.
	if w, _ := CountUnits("¿? ¡! -- «»"); w != 0 {
		t.Errorf("punctuation counted as %d words", w)
	}
	// A zero rate falls back rather than dividing by zero.
	if got := ReadingTime("one two three", ReadingRate{}); got <= 0 {
		t.Errorf("a zero rate should fall back to the default, got %v", got)
	}
	// A mixed paragraph charges each script at its own rate.
	mixed := "El texto dice 发明的文字内容 y sigue."
	w, d := CountUnits(mixed)
	if w == 0 || d == 0 {
		t.Errorf("mixed text counted as %d words and %d dense characters", w, d)
	}
}
