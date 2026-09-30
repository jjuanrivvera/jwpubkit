package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

func inventedLibrary(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func indexInvented(t *testing.T, st *store.Store, symbol, issue string, p testutil.Pub) {
	t.Helper()
	file := testutil.Build(t, t.TempDir(), p)
	unique := filepath.Join(filepath.Dir(file), symbol+"_"+issue+".jwpub")
	if err := os.Rename(file, unique); err != nil {
		t.Fatal(err)
	}
	if _, err := st.IndexLocal(unique, symbol, issue, "E"); err != nil {
		t.Fatal(err)
	}
}

func runRemaining(t *testing.T, dir string, client *cdn.Client, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var out, diagnostic bytes.Buffer
	a := &app{ctx: ctx, out: &out, err: &diagnostic, origins: map[string]string{}, cdn: client}
	t.Cleanup(func() {
		if a.st != nil {
			_ = a.st.Close()
		}
	})
	root := a.rootCmd()
	root.SetOut(&out)
	root.SetErr(&diagnostic)
	root.SetArgs(append([]string{"--library", dir, "--language", "E", "--quiet"}, args...))
	err := root.ExecuteContext(ctx)
	if diagnostic.Len() > 0 {
		t.Log(diagnostic.String())
	}
	return out.String(), err
}

func decodeRemaining[T any](t *testing.T, data string) T {
	t.Helper()
	var out T
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	return out
}

func TestDailyMonthlyAndLinkedEntries(t *testing.T) {
	st := inventedLibrary(t)
	var month strings.Builder
	for day := 1; day <= 30; day++ {
		fmt.Fprintf(&month, `<h2 data-pid="%d">Invented date %d</h2>
<p class="sa" data-pid="%d"><em>Robot theme %d.</em> <a href="jwpub://b/NWTR/1:1:1-1:1:1">Gen 1:1</a></p>
<p data-pid="%d">Clockwork comment %d. <a href="jwpub://p/E:90001/2-2">Mock citation %d</a></p>`, day*3, day, day*3+1, day, day*3+2, day, day)
	}
	p := testutil.Pub{Symbol: "es26", Undated: "es", Year: 2026, Title: "Synthetic examination",
		Docs:  []testutil.Doc{{ID: 1, MepsID: 80001, Class: 4, Title: "Synthetic month", HTML: month.String()}},
		Dated: [][4]any{{1, 20260901, 20260930, ""}, {1, 20260902, 20260902, "p/E:80001/6"}, {1, 20260903, 20260903, "p/E:80001/9-11"}}}
	indexInvented(t, st, "es", "2026", p)
	for _, day := range []int{1, 2, 3, 30} {
		t.Run(fmt.Sprint(day), func(t *testing.T) {
			data, err := runRemaining(t, st.Dir, nil, "texto", fmt.Sprintf("2026-09-%02d", day), "--json")
			if err != nil {
				t.Fatal(err)
			}
			out := decodeRemaining[dailyOut](t, data)
			if !strings.Contains(out.Theme, fmt.Sprintf("Robot theme %d.", day)) || out.Reference != "Ge 1:1" || out.Citation != fmt.Sprintf("Mock citation %d", day) {
				t.Fatalf("wrong entry: %+v", out)
			}
			if strings.Count(out.Comment, "Clockwork comment") != 1 || !strings.Contains(out.URL, fmt.Sprintf("par=%d", day*3+1)) {
				t.Fatalf("entry crossed a day boundary: %+v", out)
			}
		})
	}
	text, err := runRemaining(t, st.Dir, nil, "daily", "2026-09-30")
	if err != nil || !strings.Contains(text, "Citation: Mock citation 30") {
		t.Fatalf("text output: %s %v", text, err)
	}
	// The hint has to name a command that works: the yearly volume's symbol
	// carries the year and takes no issue, and "es --issue 2027" is a 400 from
	// the CDN.
	_, err = runRemaining(t, st.Dir, nil, "daily", "2027-01-01")
	if err == nil || !strings.Contains(err.Error(), "pubkit sync es27") {
		t.Fatalf("missing or wrong sync hint: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "--issue") {
		t.Errorf("the hint offers an issue the CDN rejects: %v", err)
	}
}

func inventedStudy(t *testing.T, st *store.Store) {
	t.Helper()
	indexInvented(t, st, "w", "202607", testutil.Pub{Symbol: "w26", Undated: "w", Year: 2026, IssueTag: 20260700, Title: "Mock study issue",
		Docs: []testutil.Doc{
			{ID: 1, MepsID: 81000, Class: 68, HTML: `<p data-pid="1"><a href="jwpub://p/E:81001/">Mock article pointer</a></p>`},
			{ID: 2, MepsID: 81001, Class: 40, Title: "Mock study article", HTML: `<p class="contextTtl" data-pid="1">Mock study week</p>
<p class="pubRefs" data-pid="2"><a href="jwpub://p/E:1102016807/">Mock opening tune 7</a></p>
<h1 data-pid="3">Mechanical squirrels</h1>
<p class="themeScrp" data-pid="4">Theme placeholder <a href="jwpub://b/NWTR/1:1:1-1:1:1">Gen 1:1</a></p>
<p data-pid="7" data-rel-pid="[20]"><span class="parNum" data-pnum="1"></span>Robot paragraph one <a href="jwpub://b/NWTR/1:2:1-1:2:2">Gen 2:1, 2</a></p>
<figure><img src="jwpub-media://robot.jpg"/><figcaption><p data-pid="8">Markup caption.</p></figcaption></figure>
<p data-pid="9" data-rel-pid="[20]"><span class="parNum" data-pnum="2"></span>Robot paragraph two.</p>
<p data-pid="10" data-rel-pid="[999]"><span class="parNum" data-pnum="3"></span>Unpaired robot.</p>
<p class="qu" data-pid="20">1, 2. Which gears turn?</p>
<aside><h2 data-pid="30">Mock review box</h2><ul><li><p data-pid="31">Which robot?</p><div class="gen-field" data-pid="32"></div></li></ul></aside>
<p class="pubRefs" data-pid="40"><a href="jwpub://p/E:1102016808/">Mock closing tune 8</a></p>`}},
		Dated: [][4]any{{1, 20260928, 20261004, "p/E:81000/1"}},
		Media: []testutil.Media{{DocID: 2, ID: 1, File: "robot.jpg", Caption: "Indexed robot caption.", BeginPID: 7}},
	})
}

func TestWatchtowerPairsSharedQuestionAndParagraphScriptures(t *testing.T) {
	st := inventedLibrary(t)
	inventedStudy(t, st)
	verseID, _ := bible.VerseID(1, 3, 1)
	if _, err := st.DB.ExecContext(t.Context(), `INSERT INTO cite(docid,pid,first,last,pub_id) SELECT 81001,9,?,?,id FROM pub WHERE symbol='w'`, verseID, verseID); err != nil {
		t.Fatal(err)
	}
	data, err := runRemaining(t, st.Dir, nil, "atalaya", "2026-10-04", "--json")
	if err != nil {
		t.Fatal(err)
	}
	out := decodeRemaining[meeting.WatchtowerArticle](t, data)
	if out.DocID != 81001 || out.Title != "Mechanical squirrels" || out.ThemeRef != "Ge 1:1" || len(out.Songs) != 2 || len(out.Paragraphs) != 3 {
		t.Fatalf("incomplete article: %+v", out)
	}
	for _, p := range out.Paragraphs[:2] {
		if p.QuestionID != 20 || p.Question != "Which gears turn?" {
			t.Fatalf("question not paired by rel-pid: %+v", p)
		}
	}
	if len(out.Paragraphs[0].Scriptures) != 1 || out.Paragraphs[0].Scriptures[0] != "Ge 2:1-2" || len(out.Paragraphs[0].Images) != 1 || out.Paragraphs[0].Images[0].Caption != "Indexed robot caption." {
		t.Fatalf("paragraph evidence: %+v", out.Paragraphs[0])
	}
	if len(out.Paragraphs[1].Scriptures) != 1 || out.Paragraphs[1].Scriptures[0] != "Ge 3:1" || out.Paragraphs[2].Question != "" || len(out.Review) != 1 {
		t.Fatalf("indexed references, missing question or review: %+v", out)
	}
	text, err := runRemaining(t, st.Dir, nil, "watchtower", "2026-09-30")
	if err != nil || !strings.Contains(text, "Box: Mock review box") || !strings.Contains(text, "Question: Which gears turn?") {
		t.Fatalf("text output: %s %v", text, err)
	}
	_, err = runRemaining(t, st.Dir, nil, "watchtower", "2026-10-05")
	if err == nil || !strings.Contains(err.Error(), "sync w issues") {
		t.Fatalf("missing-article hint: %v", err)
	}
}

func TestPlacesReportsThreeDistinctSources(t *testing.T) {
	st := inventedLibrary(t)
	indexInvented(t, st, "nwtsty", "", testutil.Pub{Symbol: "nwtsty", Undated: "nwtsty", Year: 2026, Title: "Mock Bible",
		Verses: map[int]string{0: `<span id="v1-1-1">A robot's invented verse.</span>`},
		Docs: []testutil.Doc{{ID: 1, MepsID: 82001, Class: 14, Title: "Mock appendix figure", HTML: `<ul><li><p data-pid="1">Zorbland</p></li><li><p data-pid="2">Bloopville</p></li></ul>`},
			{ID: 2, MepsID: 82002, Class: 125, Title: "Second mock appendix", HTML: `<ul><li><p data-pid="1">Blipland</p></li></ul>`},
			{ID: 3, MepsID: 82003, Class: 40, Title: "Not an appendix", HTML: `<p data-pid="1">Exclude this robot.</p>`}},
		Media: []testutil.Media{{DocID: 1, ID: 1, File: "mock.svg", Caption: "Mock figure caption."}, {DocID: 2, ID: 2, File: "second.svg"}, {DocID: 3, ID: 3, File: "exclude.svg"}},
	})
	indexInvented(t, st, "it", "", testutil.Pub{Symbol: "it", Undated: "it", Year: 2026, Title: "Mock encyclopedia",
		Docs: []testutil.Doc{{ID: 1, MepsID: 82101, Class: 13, Title: "Zorbland", HTML: `<p data-pid="1">A clockwork invented city.</p>`},
			{ID: 2, MepsID: 82102, Class: 13, Title: "Wobbleness", HTML: `<p data-pid="1">An invented abstract concept.</p>`}}})
	indexInvented(t, st, "gl", "", testutil.Pub{Symbol: "gl", Undated: "gl", Year: 2026, Title: "Mock atlas",
		Docs: []testutil.Doc{{ID: 1, MepsID: 82201, Class: 13, Title: "Mock citing map", HTML: `<p data-pid="1"><a href="jwpub://b/NWTR/1:1:1-1:1:2">Chapter citation</a></p>`},
			{ID: 2, MepsID: 82202, Class: 13, Title: "Other chapter map", HTML: `<p data-pid="1"><a href="jwpub://b/NWTR/1:2:1-1:2:1">Other citation</a></p>`}},
		Media: []testutil.Media{{DocID: 1, ID: 1, File: "atlas.jpg", Caption: "Mock map caption."}}})
	for _, query := range []string{
		`UPDATE media SET mime='image/svg+xml' WHERE pub_id=(SELECT id FROM pub WHERE symbol='nwtsty')`,
		`INSERT INTO verse_note(verse_id,seq,label,text,html,pub_id) SELECT 0,1,'Mock','Robots visited Zorbland and Wobbleness and Missingland.','',id FROM pub WHERE symbol='nwtsty'`,
		// A mismatched citation publication must not turn the other map into a match.
		`INSERT INTO cite(docid,pid,first,last,pub_id) SELECT 82202,1,0,0,id FROM pub WHERE symbol='it'`,
	} {
		if _, err := st.DB.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	data, err := runRemaining(t, st.Dir, nil, "lugares", "Gen 1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	out := decodeRemaining[placesOut](t, data)
	if len(out.Sources) != 5 {
		t.Fatalf("sources not scoped: %+v", out.Sources)
	}
	counts := map[string]int{}
	for _, row := range out.Sources {
		counts[row.Kind]++
		if row.Source == "" || row.URL == "" || row.Key == "" {
			t.Fatalf("no provenance: %+v", row)
		}
		if row.Kind == "atlas_map" && (row.DocID != 82201 || row.Images[0].Caption != "Mock map caption.") {
			t.Fatalf("atlas evidence: %+v", row)
		}
		if row.DocID == 82001 && strings.Join(row.Names, ",") != "Zorbland,Bloopville" {
			t.Fatalf("appendix labels: %+v", row)
		}
	}
	if counts["encyclopedia_term"] != 2 || counts["atlas_map"] != 1 || counts["appendix_figure"] != 2 || !strings.Contains(strings.Join(out.Notes, " "), "not classified as places") {
		t.Fatalf("unsupported classification: %+v", out)
	}
	text, err := runRemaining(t, st.Dir, nil, "places", "Gen 1")
	if err != nil || !strings.Contains(text, "Source: cite") || !strings.Contains(text, "library-wide") {
		t.Fatalf("text output: %s %v", text, err)
	}
}

func TestRemainingDateAndChapterValidation(t *testing.T) {
	for _, command := range []string{"daily", "watchtower", "update-week"} {
		for _, date := range []string{"2026-02-30", "bad-date"} {
			_, err := runRemaining(t, t.TempDir(), nil, command, date)
			if err == nil || !strings.Contains(err.Error(), "invalid date") {
				t.Fatalf("%s %s: %v", command, date, err)
			}
		}
	}
	for _, ref := range []string{"bogus", "Gen 1:99"} {
		_, err := runRemaining(t, t.TempDir(), nil, "places", ref)
		if err == nil {
			t.Fatalf("accepted %q", ref)
		}
	}
}

type remainingTransport func(*http.Request) (*http.Response, error)

func (f remainingTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func mockResponse(body string, status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func inventedWorkbook(t *testing.T, st *store.Store) {
	t.Helper()
	indexInvented(t, st, "mwb", "202609", testutil.Pub{Symbol: "mwb26", Undated: "mwb", Year: 2026, IssueTag: 20260900, Title: "Mock workbook",
		Docs: []testutil.Doc{{ID: 1, MepsID: 83001, Class: 106, Title: "Mock week", HTML: `<h1 data-pid="1">Mock week</h1>
<div class="dc-icon--sheep"><h2 data-pid="2">Mock section</h2></div>
<h3 data-pid="3">1. Robot videos</h3>
<p data-pid="4"><a href="https://www.jw.org/finder?lank=pub-robot_1_VIDEO">Robot one</a>
<a href="https://www.jw.org/finder?lank=pub-robot_2_VIDEO">Robot two</a>
<a href="https://www.jw.org/finder?lank=pub-robot_3_VIDEO">Robot three</a></p>`}},
		Dated: [][4]any{{1, 20260928, 20261004, "p/E:83001/1-4"}}})
}

func mockUpdateClient(t *testing.T, st *store.Store, failSymbol string) (*cdn.Client, *int) {
	t.Helper()
	pubs, err := st.Pubs()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]store.Pub{}
	for _, p := range pubs {
		byKey[p.Key] = p
	}
	requests := new(int)
	return &cdn.Client{Lang: "E", HTTP: &http.Client{Transport: remainingTransport(func(req *http.Request) (*http.Response, error) {
		*requests++
		if req.URL.Query().Get("fileformat") == "JWPUB" {
			symbol, issue := req.URL.Query().Get("pub"), req.URL.Query().Get("issue")
			if symbol == failSymbol {
				return mockResponse(`{}`, http.StatusNotFound), nil
			}
			p := byKey[store.PubKey(symbol, "E", issue)]
			if p.Key == "" {
				t.Errorf("unexpected publication request: %s", req.URL)
			}
			body, _ := json.Marshal(map[string]any{"files": map[string]any{"E": map[string]any{"JWPUB": []any{map[string]any{"filesize": p.Size, "file": map[string]any{"url": "https://mock.invalid/" + filepath.Base(p.File), "checksum": p.MD5}}}}}})
			return mockResponse(string(body), http.StatusOK), nil
		}
		if req.URL.Host == "mock.invalid" {
			if strings.HasSuffix(req.URL.Path, ".jwpub") {
				for _, p := range byKey {
					if filepath.Base(p.File) == filepath.Base(req.URL.Path) {
						data, err := os.ReadFile(p.File)
						if err != nil {
							return nil, err
						}
						return mockResponse(string(data), http.StatusOK), nil
					}
				}
				t.Fatalf("unexpected download: %s", req.URL)
			}
			return mockResponse("WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nA robot transcript.\n", http.StatusOK), nil
		}
		if strings.Contains(req.URL.Path, "pub-robot_3_VIDEO") {
			return mockResponse(`{"media":[{"title":"Mock robot three","files":[{"subtitles":{"url":"https://mock.invalid/robot.vtt"}}]}]}`, http.StatusOK), nil
		}
		return mockResponse(`{"media":[{"title":"Mock silent robot","files":[]}],"files":{"E":{"MP4":[]}}}`, http.StatusOK), nil
	})}}, requests
}

func TestUpdateWeekDryRunDoesNotCreateLibraryOrUseNetwork(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	client := &cdn.Client{Lang: "E", HTTP: &http.Client{Transport: remainingTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("dry run used the network")
		return nil, nil
	})}}
	data, err := runRemaining(t, dir, client, "semanal", "2026-09-30", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	out := decodeRemaining[weekUpdateOut](t, data)
	if !out.DryRun || out.Monday != "2026-09-28" || len(out.Rows) != 3 || len(out.Notes) != 1 {
		t.Fatalf("incomplete plan: %+v", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dry run created a library: %v", err)
	}
}

func TestUpdateWeekCurrentPublicationsAndNonfatalVideoFailures(t *testing.T) {
	st := inventedLibrary(t)
	inventedWorkbook(t, st)
	inventedStudy(t, st)
	indexInvented(t, st, "w", "202608", testutil.Pub{Symbol: "w26", Undated: "w", Year: 2026, IssueTag: 20260800, Title: "Mock next study issue",
		Docs: []testutil.Doc{{ID: 1, MepsID: 84001, Class: 68, HTML: `<p data-pid="1">Mock contents.</p>`}}})
	if err := st.PutVideo("pub-robot_1_VIDEO", "E", "Cached robot", 2, "mock://cached", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(st.Dir, subtitleDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir, subtitleDir, "pub-robot_1_VIDEO.E.vtt"), []byte("WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nCached robot words.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, requests := mockUpdateClient(t, st, "")
	plan, err := runRemaining(t, st.Dir, client, "update-week", "2026-09-30", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if *requests != 0 || len(decodeRemaining[weekUpdateOut](t, plan).Rows) != 6 {
		t.Fatalf("dry run used network or missed videos: %d %s", *requests, plan)
	}
	data, err := runRemaining(t, st.Dir, client, "update-week", "2026-09-30", "--json")
	if err != nil {
		t.Fatal(err)
	}
	out := decodeRemaining[weekUpdateOut](t, data)
	if len(out.Rows) != 6 {
		t.Fatalf("missing results: %+v", out)
	}
	for _, row := range out.Rows[:4] {
		if row.Status != "already current" {
			t.Fatalf("current copy was not skipped: %+v", row)
		}
	}
	if out.Rows[4].Status != "failed" || out.Rows[4].Error == "" || out.Rows[5].Status != "synced" {
		t.Fatalf("video failure stopped a later success: %+v", out)
	}
	if n, err := st.CueVideoCount("E"); err != nil || n != 2 {
		t.Fatalf("transcripts were not indexed: %d %v", n, err)
	}
	text, err := runRemaining(t, st.Dir, client, "update-week", "2026-09-30")
	if err != nil || !strings.Contains(text, "mwb_E_202609: already current") || !strings.Contains(text, "pub-robot_2_VIDEO: failed") {
		t.Fatalf("text report: %s %v", text, err)
	}
	client, _ = mockUpdateClient(t, st, "w")
	data, err = runRemaining(t, st.Dir, client, "update-week", "2026-09-30", "--json")
	if err == nil || !strings.Contains(err.Error(), "2 of 3 publications failed") || len(decodeRemaining[weekUpdateOut](t, data).Rows) != 6 {
		t.Fatalf("publication failure was not reported after all work: %s %v", data, err)
	}
}

func TestPlacesEmptyLibrary(t *testing.T) {
	st := inventedLibrary(t)
	data, err := runRemaining(t, st.Dir, nil, "places", "Ps 3", "--json")
	if err != nil || len(decodeRemaining[placesOut](t, data).Notes) != 5 {
		t.Fatalf("empty library: %s %v", data, err)
	}
	data, err = runRemaining(t, st.Dir, nil, "places", "Ps 1", "--json")
	if err != nil || len(decodeRemaining[placesOut](t, data).Sources) != 0 {
		t.Fatalf("Psalm without superscription: %s %v", data, err)
	}
}

func TestDailyLinkedRangeAndHeadingBoundaries(t *testing.T) {
	parsed, err := content.Parse(`<h2 data-pid="3">機械の日付</h2>
<p data-pid="4">First robot theme.</p><p data-pid="5">First robot comment.</p>
<h2 data-pid="6">تاريخ الروبوت</h2><p data-pid="7">Second robot theme.</p><p data-pid="8">Second robot comment.</p>`)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for _, link := range []string{"p/E:80001/3", "jwpub://p/E:80001/3-5"} {
		got := dailyBlocks(parsed, store.DatedDoc{First: 20260101, Last: 20260101, Link: link}, day)
		if len(got) != 3 || got[0].PID != 3 || got[2].PID != 5 {
			t.Fatalf("%s crossed a heading boundary: %+v", link, got)
		}
	}
	if dailyLink("not-a-link") != nil {
		t.Fatal("accepted an invalid dated link")
	}
}

func TestUpdateWeekDownloadsBeforeDiscoveringVideoKeys(t *testing.T) {
	source := inventedLibrary(t)
	inventedWorkbook(t, source)
	inventedStudy(t, source)
	indexInvented(t, source, "w", "202608", testutil.Pub{Symbol: "w26", Undated: "w", Year: 2026, IssueTag: 20260800, Title: "Mock next issue",
		Docs: []testutil.Doc{{ID: 1, MepsID: 84001, Class: 68, HTML: `<p data-pid="1">Mock robot contents.</p>`}}})
	client, _ := mockUpdateClient(t, source, "")
	dir := t.TempDir()
	data, err := runRemaining(t, dir, client, "update-week", "2026-09-30", "--json")
	if err != nil {
		t.Fatal(err)
	}
	out := decodeRemaining[weekUpdateOut](t, data)
	if len(out.Rows) != 6 || out.Rows[5].Status != "synced" {
		t.Fatalf("new workbook videos were not fetched: %+v", out)
	}
	for _, row := range out.Rows[:3] {
		if row.Status != "synced" {
			t.Fatalf("publication was not synced: %+v", row)
		}
	}
	st, err := store.Open(dir, "E")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if dd, err := (&meeting.Builder{Store: st}).FindWorkbook(time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)); err != nil || dd == nil {
		t.Fatalf("downloaded workbook was not indexed: %+v %v", dd, err)
	}
}
