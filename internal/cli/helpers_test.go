package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func TestPlural2(t *testing.T) {
	for n, want := range map[int]string{0: "s", 1: "", 2: "s", 17: "s"} {
		if got := plural2(n); got != want {
			t.Errorf("plural2(%d) = %q, want %q", n, got, want)
		}
	}
}

// truncate counts runes, not bytes: cutting a title at a byte boundary can land
// in the middle of a character and print a replacement glyph.
func TestTruncate(t *testing.T) {
	cases := []struct {
		in, want string
		n        int
	}{
		{"short", "short", 10},
		{"exactly-10", "exactly-10", 10},
		{"a longer title than that", "a longer t…", 10},
		{"año tras año y más", "año tra…", 7},
		{"日本語の題名です", "日本語…", 3},
		{"", "", 5},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.n); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestFoldASCII(t *testing.T) {
	if got := foldASCII("Éxodo, Números y Corintios"); got != "Exodo, Numeros y Corintios" {
		t.Errorf("foldASCII = %q", got)
	}
	// Only the mappings it declares are folded; anything else survives intact
	// rather than being mangled into something unreadable.
	if got := foldASCII("Πράξεις"); got != "Πράξεις" {
		t.Errorf("a Greek name should pass through untouched, got %q", got)
	}
}

// The slug goes into a filename, so it must never carry a path separator —
// book names come from the publication, in whatever language it is in.
func TestSafeInFilename(t *testing.T) {
	for in, want := range map[string]string{
		"Gen":       "Gen",
		"1/2Reyes":  "1-2Reyes",
		`a:b*c?d"e`: "a-b-c-d-e",
		"x<y>z|w":   "x-y-z-w",
		"tab\there": "tab-here",
		"Быт":       "Быт",
	} {
		if got := safeInFilename(in); got != want {
			t.Errorf("safeInFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChapterSlug(t *testing.T) {
	bible.UseLanguage("S")
	t.Cleanup(func() { bible.UseLanguage("E") })
	got := chapterSlug(2) // Exodus: accented and two words in some forms
	if strings.ContainsAny(got, ` /\:`) {
		t.Errorf("chapterSlug(2) = %q; it goes into a filename", got)
	}
	if got == "" {
		t.Error("chapterSlug should name the book, not return nothing")
	}
	// An unknown book number must not panic or produce a path.
	if s := chapterSlug(999); strings.ContainsAny(s, `/\`) {
		t.Errorf("chapterSlug(999) = %q", s)
	}
}

func TestLeadingDigits(t *testing.T) {
	for in, want := range map[string]string{
		"1102023801_univ_art": "1102023801",
		"nwtsty_E_1":          "",
		"2026":                "2026",
		"":                    "",
		"12-34":               "12",
	} {
		if got := leadingDigits(in); got != want {
			t.Errorf("leadingDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContainsAndContainsInt(t *testing.T) {
	xs := []string{"a.jwpub", "b.jwpub"}
	if !contains(xs, "b.jwpub") || contains(xs, "c.jwpub") || contains(nil, "a") {
		t.Error("contains is wrong")
	}
	ns := []int{3, 7}
	if !containsInt(ns, 7) || containsInt(ns, 8) || containsInt(nil, 1) {
		t.Error("containsInt is wrong")
	}
}

func TestKB(t *testing.T) {
	for n, want := range map[int]string{0: "0 KB", 1024: "1 KB", 1536: "2 KB", 10240: "10 KB"} {
		if got := kb(n); got != want {
			t.Errorf("kb(%d) = %q, want %q", n, got, want)
		}
	}
}

// dims must read the size out of the bytes, and answer 0x0 rather than fail on
// something that is not an image: a CDN can hand back an HTML error page.
func TestDims(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 7, 3))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if w, h := dims(buf.Bytes()); w != 7 || h != 3 {
		t.Errorf("dims = %dx%d, want 7x3", w, h)
	}
	if w, h := dims([]byte("<html>404</html>")); w != 0 || h != 0 {
		t.Errorf("dims of a non-image = %dx%d, want 0x0", w, h)
	}
	if w, h := dims(nil); w != 0 || h != 0 {
		t.Errorf("dims of nothing = %dx%d", w, h)
	}
}

func TestRateOr(t *testing.T) {
	if got := rateOr(0, 120); got != 120 {
		t.Errorf("rateOr(0, 120) = %v, want the fallback", got)
	}
	if got := rateOr(-5, 120); got != 120 {
		t.Errorf("a negative rate is not a rate: got %v", got)
	}
	if got := rateOr(95, 120); got != 95 {
		t.Errorf("rateOr(95, 120) = %v, want what was given", got)
	}
}

func TestCDNImageURLs(t *testing.T) {
	got := cdnImageURLs("1102023801_univ_art.jpg", "E")
	if len(got) != 2 {
		t.Fatalf("want xl and lg, got %d: %+v", len(got), got)
	}
	if got[0].size != "xl" || got[1].size != "lg" {
		t.Errorf("xl must come first — lg is only the fallback: %+v", got)
	}
	const wantXL = "https://cms-imgp.jw-cdn.org/img/p/1102023801/univ/art/1102023801_univ_art_xl.jpg"
	if got[0].url != wantXL {
		t.Errorf("xl url = %q, want %q", got[0].url, wantXL)
	}

	// A name that carries the language takes the language segment, not univ.
	if u := cdnImageURLs("1102023801_S_art.jpg", "S"); len(u) == 0 || !strings.Contains(u[0].url, "/S/art/") {
		t.Errorf("a language-specific image should use the language segment: %+v", u)
	}
	// …but only for the language asked for.
	if u := cdnImageURLs("1102023801_S_art.jpg", "E"); len(u) == 0 || !strings.Contains(u[0].url, "/univ/art/") {
		t.Errorf("another language's image is not ours: %+v", u)
	}

	// No docid to build the path from, and hyphenated names (which are not
	// cms-imgp's shape) give nothing rather than a URL that 404s.
	if u := cdnImageURLs("nwtsty_E_art.jpg", "E"); u != nil {
		t.Errorf("a name with no leading docid should give nothing, got %+v", u)
	}
	if u := cdnImageURLs("1102023801-art.jpg", "E"); u != nil {
		t.Errorf("a hyphenated name should give nothing, got %+v", u)
	}
}

func TestMergeCitations(t *testing.T) {
	have := []store.Citation{{DocID: 1, PIDs: []int{3, 4}}, {DocID: 2, PIDs: []int{9}}}
	add := []store.Citation{
		{DocID: 1, PIDs: []int{4, 5}}, // 4 is already there, 5 is new
		{DocID: 3, PIDs: []int{1}},    // a document not seen before
	}
	got := mergeCitations(have, add)
	if len(got) != 3 {
		t.Fatalf("want 3 documents, got %d: %+v", len(got), got)
	}
	if len(got[0].PIDs) != 3 || !containsInt(got[0].PIDs, 5) {
		t.Errorf("document 1 should gain paragraph 5 once: %+v", got[0].PIDs)
	}
	if got[2].DocID != 3 {
		t.Errorf("the new document should be appended: %+v", got)
	}
	// Merging into nothing is the same as taking what was added.
	if out := mergeCitations(nil, add); len(out) != 2 {
		t.Errorf("mergeCitations(nil, add) = %+v", out)
	}
}

func TestMsAndMb(t *testing.T) {
	if got := ms(250_000_000); got != "250 ms" {
		t.Errorf("ms(250ms) = %q", got)
	}
	if got := ms(1_500_000_000); got != "1.5 s" {
		t.Errorf("ms(1.5s) = %q", got)
	}
	if got := mb(2_500_000); got != "2.5 MB" {
		t.Errorf("mb = %q", got)
	}
}
