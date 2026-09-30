// Command demolib builds a synthetic JWPUB library, so the demo recording can
// show the tool working without showing anything from a publication.
//
// A demo of this tool that recorded real output would put publication text in a
// picture, inside a public repository, where it cannot even be argued to be
// structure. Every word on screen here is invented; the shapes, the classes and
// the links are the real ones, which is why the commands behave normally.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

func main() {
	dir := flag.String("library", "", "directory to build the synthetic library in (required)")
	lang := flag.String("language", "E", "language symbol to file it under")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: demolib --library <dir> [--language E]")
		os.Exit(2)
	}
	if err := run(*dir, *lang); err != nil {
		fmt.Fprintln(os.Stderr, "demolib:", err)
		os.Exit(1)
	}
}

func run(dir, lang string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "demolib-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	st, err := store.Open(dir, lang)
	if err != nil {
		return err
	}
	defer st.Close()

	for _, p := range []struct {
		symbol, issue string
		pub           testutil.Pub
	}{
		{"nwtsty", "", inventedBible()},
		{"mwb", "202601", inventedWorkbook()},
		{"wcg", "", inventedBook()},
	} {
		path, err := testutil.BuildE(work, p.pub)
		if err != nil {
			return err
		}
		stats, err := st.IndexLocal(path, p.symbol, p.issue, lang)
		if err != nil {
			return fmt.Errorf("indexing %s: %w", p.symbol, err)
		}
		fmt.Printf("%-10s %s\n", p.symbol, stats)
	}
	fmt.Printf("\nsynthetic library ready in %s\n", filepath.Clean(dir))
	return nil
}

// inventedBible is a Bible with a handful of verses, their footnotes, marginal
// references and study notes — all invented.
func inventedBible() testutil.Pub {
	verses := map[int]string{}
	for v := 1; v <= 6; v++ {
		id, ok := bible.VerseID(1, 1, v)
		if !ok {
			continue
		}
		verses[id] = fmt.Sprintf(
			`<span id="v1-1-%d" class="v">Invented verse %d of the sample chapter, written for a demo.</span>`, v, v)
	}
	return testutil.Pub{
		Symbol: "nwtsty", Undated: "nwtsty", Year: 2026, Title: "Sample Bible (invented)",
		Verses: verses,
	}
}

func inventedWorkbook() testutil.Pub {
	week := `<header><h1 id="p1" data-pid="1">5-11 JANUARY</h1>
<h2 id="p2" data-pid="2"><a href="jwpub://b/NWTR/1:1:1-1:1:31" class="b"><strong>SAMPLE 1</strong></a></h2></header>
<div class="bodyTxt">
<h3 id="p3" data-pid="3"><a class="xt" href="jwpub://p/E:1102016801/"><strong>Song 1</strong></a> <strong>and prayer | Opening part</strong> <span>(1 min.)</span></h3>
<div class="dc-icon--gem dc-icon-layout--top"><h2 id="p4" data-pid="4"><strong>FIRST SECTION</strong></h2></div>
<h3 id="p5" data-pid="5"><strong>1. An invented part</strong></h3>
<div><p id="p6" data-pid="6">(10 mins.)</p></div>
<p id="p7" data-pid="7">An invented idea (<a href="jwpub://b/NWTR/1:1:1-1:1:1" class="b">Sample 1:1</a>).</p>
<h3 id="p12" data-pid="12"><strong>2. Bible Reading</strong></h3>
<div><p id="p13" data-pid="13">(4 mins.) <a href="jwpub://b/NWTR/1:1:1-1:1:6" class="b">Sample 1:1-6</a></p></div>
<div class="dc-icon--sheep dc-icon-layout--top"><h2 id="p14" data-pid="14"><strong>SECOND SECTION</strong></h2></div>
<h3 id="p15" data-pid="15"><strong>3. Congregation Bible Study</strong></h3>
<div><p id="p16" data-pid="16">(30 mins.) <a class="xt" href="jwpub://p/E:1102025901/">chapter 1</a></p></div>
</div>`
	chapter := `<header><p class="contextTtl" id="p1" data-pid="1"><strong>1</strong> SAMPLE</p>
<h1 id="p2" data-pid="2"><strong>An invented chapter</strong></h1></header>
<div class="bodyTxt"><p id="p3" data-pid="3">An invented paragraph of the study chapter.</p>
<h3 id="p4" data-pid="4"><strong>Questions</strong></h3>
<p id="p5" data-pid="5"><strong>An invented question?</strong></p><div class="gen-field" id="p6" data-pid="6"></div>
</div>`
	return testutil.Pub{
		Symbol: "mwb26", Undated: "mwb", Year: 2026, IssueTag: 20260100, Title: "Sample workbook (invented)",
		Docs:  []testutil.Doc{{ID: 1, MepsID: 202026001, Class: 106, Title: "5-11 January", HTML: week}},
		Dated: [][4]any{{1, 20260105, 20260111, "p/E:202026001/1-16"}},
		Extracts: []testutil.Extract{
			{DocID: 1, ExtractID: 1, Link: "p/E:1102016801/", Caption: "sjj song 1", HTML: "<p>invented</p>",
				RefDocID: 1102016801, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 3, Sort: 1},
			{DocID: 1, ExtractID: 2, Link: "p/E:1102025901/", Caption: "wcg pp. 4-9", HTML: chapter,
				RefDocID: 1102025901, RefClass: 13, RefSymbol: "wcg", RefUndated: "wcg", BeginPID: 16, Sort: 2},
		},
	}
}

func inventedBook() testutil.Pub {
	return testutil.Pub{
		Symbol: "wcg", Undated: "wcg", Year: 2026, Title: "Sample study book (invented)",
		Docs: []testutil.Doc{
			{ID: 1, MepsID: 1102025901, Class: 13, Title: "An invented chapter",
				HTML: `<h1 id="p1" data-pid="1">An invented chapter</h1>
<p id="p2" data-pid="2"><span class="parNum" data-pnum="1"></span>Courage is the invented subject of this paragraph, written for a demo (<a href="jwpub://b/NWTR/1:1:1-1:1:1" class="b">Sample 1:1</a>).</p>
<p id="p3" data-pid="3"><span class="parNum" data-pnum="2"></span>A second invented paragraph about courage and comfort.</p>`},
		},
	}
}
