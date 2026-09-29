package content

import "testing"

func TestCite(t *testing.T) {
	cases := []struct {
		name                        string
		published, symbol, issue    string
		page, paragraph, docid, pid int
		want                        string
	}{{
		name:      "a publication's own wording wins over anything composed",
		published: "w13 15/1 pág. 9", symbol: "w", issue: "20130115",
		want: "w13 15/1 pág. 9",
	}, {
		name: "the monthly form", symbol: "w", issue: "202405", page: 8, paragraph: 3,
		want: "w24.05 p. 8 par. 3",
	}, {
		name: "the pre-2016 dated form", symbol: "w", issue: "20130115", page: 9,
		want: "w13 15/1 p. 9",
	}, {
		name: "a symbol that already carries the year", symbol: "mwb26", issue: "202609", page: 8,
		want: "mwb26.09 p. 8",
	}, {
		name: "a book has no issue", symbol: "it", page: 1200, paragraph: 2,
		want: "it p. 1200 par. 2",
	}, {
		name: "nothing but a symbol", symbol: "wcg",
		want: "wcg",
	}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Cite(c.published, c.symbol, c.issue, c.page, c.paragraph, c.docid, c.pid)
			if got.Text != c.want {
				t.Errorf("Text = %q, want %q", got.Text, c.want)
			}
		})
	}
}

func TestCiteCarriesAnAddress(t *testing.T) {
	got := Cite("", "w", "202405", 8, 3, 2024567, 12)
	if got.URL == "" {
		t.Error("a citation should be openable")
	}
}

// Every address this tool prints has to point at the language the reader asked
// for. It used to hardcode one: a per-language wol path that cannot be derived
// from a JWPUB. The finder form takes the document id and the language symbol,
// which are both already known.
func TestAddressesFollowTheLanguage(t *testing.T) {
	t.Cleanup(func() { UseLanguage("") })

	UseLanguage("")
	if got := DocURL(1102025901, 5); got != "https://www.jw.org/finder?docid=1102025901&par=5" {
		t.Errorf("with no language set: %s", got)
	}
	for _, lang := range []string{"S", "E", "J"} {
		UseLanguage(lang)
		want := "https://www.jw.org/finder?docid=1102025901&par=5&wtlocale=" + lang
		if got := DocURL(1102025901, 5); got != want {
			t.Errorf("DocURL in %s = %s, want %s", lang, got, want)
		}
		if got := FinderURL("pub-x_1_VIDEO"); got != "https://www.jw.org/finder?lank=pub-x_1_VIDEO&wtlocale="+lang {
			t.Errorf("FinderURL in %s = %s", lang, got)
		}
	}
	// A document with no paragraph carries no paragraph.
	UseLanguage("S")
	if got := DocURL(42, 0); got != "https://www.jw.org/finder?docid=42&wtlocale=S" {
		t.Errorf("DocURL without a paragraph = %s", got)
	}
}
