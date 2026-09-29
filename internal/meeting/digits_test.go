package meeting

import "testing"

// The scripts below are the ones that break an ASCII-only reader: Arabic and
// Persian have their own digits, Devanagari and Bengali too, and CJK text uses
// fullwidth punctuation after an ASCII digit.
func TestFirstNumber(t *testing.T) {
	cases := []struct {
		name, in string
		want     int
		ok       bool
	}{
		{"latin", "(10 mins.)", 10, true},
		{"arabic-indic", "(١٠ دقائق)", 10, true},
		{"eastern arabic-indic", "(۱۰ دقیقه)", 10, true},
		{"devanagari", "(१० मिनट)", 10, true},
		{"bengali", "(১০ মিনিট)", 10, true},
		{"fullwidth", "（１０分）", 10, true},
		{"three digits", "١٢٨", 128, true},
		{"stops at the first run", "10 mins. 5", 10, true},
		{"no digits at all", "sin número", 0, false},
		{"a real zero", "0", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := firstNumber(c.in)
			if got != c.want || ok != c.ok {
				t.Errorf("firstNumber(%q) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestLeadingNumber(t *testing.T) {
	cases := []struct {
		name, in  string
		wantNum   int
		wantRest  string
		wantFound bool
	}{
		{"latin", "1. A title", 1, "A title", true},
		{"fullwidth stop", "1．标题", 1, "标题", true},
		{"arabic with a mark", "١- \u200fعنوان", 1, "عنوان", true},
		{"two digits", "10. Another", 10, "Another", true},
		{"not numbered", "A title", 0, "A title", false},
		{"an opening quote is part of the title", "4. “A video”", 4, "“A video”", true},
		{"a dash separator", "2 - A title", 2, "A title", true},
		{"only a number is not a part", "7", 0, "7", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, rest, ok := leadingNumber(c.in)
			if n != c.wantNum || rest != c.wantRest || ok != c.wantFound {
				t.Errorf("leadingNumber(%q) = %d, %q, %v; want %d, %q, %v",
					c.in, n, rest, ok, c.wantNum, c.wantRest, c.wantFound)
			}
		})
	}
}
