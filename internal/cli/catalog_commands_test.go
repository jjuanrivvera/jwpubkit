package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/jwpub"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

func TestCatalogAndSubtitleCommands(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/categories/E":
			_, _ = io.WriteString(w, `{"category":{"subcategories":[{"key":"invented-a"},{"key":"invented-b"}]}}`)
		case "/categories/E/invented-a":
			_, _ = io.WriteString(w, `{"category":{"name":"Invented A","subcategories":[{"key":"invented-b"}],"media":[{"languageAgnosticNaturalKey":"pub-invented_1_VIDEO","title":"Invented lighthouse machine","type":"video","files":[{"subtitles":{"url":"`+"http://"+r.Host+`/vtt"}}]}]}}`)
		case "/categories/E/invented-b":
			_, _ = io.WriteString(w, `{"category":{"media":[{"languageAgnosticNaturalKey":"pub-invented_1_VIDEO","type":"video"},{"languageAgnosticNaturalKey":"pub-invented_2_VIDEO","type":"video"}]}}`)
		case "/vtt":
			calls.Add(1)
			_, _ = io.WriteString(w, "WEBVTT\n\n00:01.000 --> 00:02.000\nlighthouse\n\n00:02.000 --> 00:03.000\nmachine hums purple\n")
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := cdn.New("E")
	client.HTTP = server.Client()
	client.CategoriesURL = server.URL + "/categories"
	dir := t.TempDir()
	data, err := runRemaining(t, dir, client, "catalog", "media", "--refresh", "--interval", "0", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "no_vtt") || !strings.Contains(data, "Invented lighthouse") {
		t.Fatal(data)
	}
	for i := 0; i < 2; i++ {
		data, err = runRemaining(t, dir, client, "subtitles", "sync", "--catalog", "--no-video", "--interval", "0", "--json")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(data, "indexed") {
			t.Fatal(data)
		}
	}
	if _, err := runRemaining(t, dir, client, "catalog", "media", "--refresh", "--resume", "--interval", "0"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("resume downloaded %d times", calls.Load())
	}
	data, err = runRemaining(t, dir, client, "subtitles", "find", `"lighthouse machine"`, "--scope", "catalog", "--context", "15s", "--json")
	if err != nil || !strings.Contains(data, "machine hums purple") || !strings.Contains(data, "timestamp") {
		t.Fatalf("%s %v", data, err)
	}
	data, err = runRemaining(t, dir, client, "catalog", "media", "--offline", "--json")
	if err != nil || !strings.Contains(data, "indexed") {
		t.Fatalf("%s %v", data, err)
	}
	for _, args := range [][]string{{"subtitles", "sync"}, {"subtitles", "sync", "--catalog", "--no-video", "--budget-bytes", "0"}, {"subtitles", "find", "x", "--scope", "bad"}, {"subtitles", "find", "x", "--context", "-1s"}, {"catalog", "media", "--refresh", "--offline"}, {"catalog", "media", "--refresh", "--interval", "-1s"}} {
		if _, err := runRemaining(t, dir, client, args...); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestCatalogImportAndSyncBudget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(t.TempDir(), "catalog.json")
	cat := mediaCatalog{Language: "E", Media: []cdn.MediaItem{{LanguageAgnosticNaturalKey: "pub-invented_1_VIDEO", Type: "video"}}}
	b, err := json.Marshal(cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runRemaining(t, dir, nil, "catalog", "media", "--file", file); err != nil {
		t.Fatal(err)
	}
	if _, err := runRemaining(t, t.TempDir(), nil, "catalog", "media"); err == nil {
		t.Fatal("absent cache accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"pubName":"Invented","files":{"E":{"PDF":[{"filesize":13}],"JWPUB":[{"filesize":200,"file":{"url":"http://invented.invalid/archive.jwpub","checksum":"abc"}}]}}}`)
	}))
	defer server.Close()
	client := cdn.New("E")
	client.HTTP = server.Client()
	client.PubMediaURL = server.URL
	absent := filepath.Join(t.TempDir(), "absent")
	data, err := runRemaining(t, absent, client, "sync", "invented", "--plan", "--interval", "0", "--json")
	if err != nil || !strings.Contains(data, `"download_bytes": 200`) {
		t.Fatalf("%s %v", data, err)
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatal("plan wrote library")
	}
	data, err = runRemaining(t, dir, client, "sync", "invented", "--budget-bytes", "1", "--interval", "0", "--json")
	if err == nil || !strings.Contains(data, "pending") {
		t.Fatalf("%s %v", data, err)
	}
	data, err = runRemaining(t, dir, client, "catalog", "publications", "invented", "--year", "2026", "--format", "PDF", "--interval", "0", "--json")
	if err != nil || !strings.Contains(data, "202612") || strings.Contains(data, "JWPUB") {
		t.Fatalf("%s %v", data, err)
	}
	for _, args := range [][]string{{"catalog", "publications"}, {"catalog", "publications", "x", "--year", "12"}, {"catalog", "publications", "x", "--year", "2026", "--issue", "202601"}, {"catalog", "publications", "x", "--symbol", "y"}, {"catalog", "publications", "x", "--offline"}, {"sync", "x", "--plan", "--file", "bad"}} {
		if _, err := runRemaining(t, dir, client, args...); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestClipTimesAndReferences(t *testing.T) {
	for _, value := range []string{"12", "08:12", "01:08:12.5"} {
		if d, err := clipTime(value); err != nil || d < time.Second {
			t.Fatalf("%q %v", value, err)
		}
	}
	for _, value := range []string{"-1", "1:99", "bad", "1:2:3:4"} {
		if _, err := clipTime(value); err == nil {
			t.Fatal(value)
		}
	}
	symbol, issue := referencePublication(store.Extract{RefSymbol: "invented26", RefIssue: 20260100})
	if symbol != "invented" || issue != "202601" {
		t.Fatalf("%s %s", symbol, issue)
	}
	symbol, _ = referencePublication(store.Extract{RefSymbol: "invented19"})
	if symbol != "invented19" {
		t.Fatal(symbol)
	}
}

func TestImageSelectionSVGAndPlacesRanges(t *testing.T) {
	st := inventedLibrary(t)
	indexInvented(t, st, "gl", "", testutil.Pub{Symbol: "gl", Title: "Invented blue atlas", Docs: []testutil.Doc{{ID: 1, MepsID: 86001, Class: 13, Title: "Invented gear map", HTML: `<p data-pid="5"><span class="parNum" data-pnum="3">3</span> A toy village. <a href="jwpub://b/NWTR/1:1:1-1:1:2">Invented citation</a></p>`}}, Media: []testutil.Media{{DocID: 1, ID: 1, File: "invented.svg", BeginPID: 5, Caption: "Invented map."}}, ImageName: "invented.svg", ImageData: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><rect width="200" height="100"/></svg>`)})
	if _, err := st.DB.ExecContext(t.Context(), `UPDATE media SET mime='image/svg+xml'`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	data, err := runRemaining(t, st.Dir, nil, "image", "86001", "--offline", "--output", dir, "--genre", "svg", "--figure", "invented.svg", "--json")
	if err != nil || !strings.Contains(data, `"width": 200`) || !strings.Contains(data, "DocumentMultimedia") {
		t.Fatalf("%s %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "invented.svg")); err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]string{{"--paragraph", "3"}, {"--passage", "Gen 1:1"}} {
		args := append([]string{"image", "86001", "--list", "--json"}, selection...)
		data, err = runRemaining(t, st.Dir, nil, args...)
		if err != nil || !strings.Contains(data, "invented.svg") {
			t.Fatalf("%s %v", data, err)
		}
	}
	for _, selection := range [][]string{{"--paragraph", "bad"}, {"--paragraph", "3-1"}, {"--genre", "bad"}, {"--passage", "bad"}, {"--paragraph", "3", "--passage", "Gen 1"}} {
		args := append([]string{"image", "86001", "--list"}, selection...)
		if _, err := runRemaining(t, st.Dir, nil, args...); err == nil {
			t.Fatalf("accepted %v", selection)
		}
	}
	data, err = runRemaining(t, st.Dir, nil, "places", "Gen 1:1-2:2", "--kind", "map", "--figure", "invented.svg", "--json")
	if err != nil || !strings.Contains(data, `"classification": "map"`) {
		t.Fatalf("%s %v", data, err)
	}
	if w, h := svgDims([]byte(`<svg width="10px" height="20px"/>`)); w != 10 || h != 20 {
		t.Fatalf("%d %d", w, h)
	}
	for _, bad := range []string{"invalid", "<rect/>", `<svg viewBox="bad"/>`} {
		if w, h := svgDims([]byte(bad)); w != 0 || h != 0 {
			t.Fatalf("%s %d %d", bad, w, h)
		}
	}
	archiveFile := testutil.Build(t, t.TempDir(), testutil.Pub{Symbol: "invented", Title: "Invented dependency", ImageName: "part.png", ImageData: []byte("invented pixels")})
	archive, err := jwpub.OpenContents(archiveFile)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	b, deps, err := inlineSVG([]byte(`<svg><image href="part.png"/><use href="#mark"/></svg>`), "invented.svg", archive)
	if err != nil || len(deps) != 1 || !strings.Contains(string(b), "data:image/png;base64") {
		t.Fatalf("%s %v %v", b, deps, err)
	}
	for _, ref := range []string{"missing.png", "https://example.invalid/p.png"} {
		if _, _, err := inlineSVG([]byte(`<svg><image href="`+ref+`"/></svg>`), "invented.svg", archive); err == nil {
			t.Fatal(ref)
		}
	}
}
func TestWeekReferencesDryRunAndSync(t *testing.T) {
	st := inventedLibrary(t)
	indexInvented(t, st, "mwb", "202609", testutil.Pub{Symbol: "mwb26", Undated: "mwb", Year: 2026, IssueTag: 20260900, Title: "Invented schedule", Docs: []testutil.Doc{{ID: 1, MepsID: 87001, Class: 106, Title: "Invented week", HTML: `<p data-pid="1">Invented gears.</p>`}}, Dated: [][4]any{{1, 20260928, 20261004, "invented"}}, Extracts: []testutil.Extract{{DocID: 1, ExtractID: 1, RefDocID: 87002, RefSymbol: "invented19", BeginPID: 1, Link: "p/E:87002/1-1", HTML: `<p data-pid="1">Invented missing gear reference.</p>`}}})
	day := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	app := &app{ctx: t.Context(), st: st, lang: "E", libDir: st.Dir, out: io.Discard, err: io.Discard}
	out := &weekUpdateOut{}
	if err := app.updateReferences(st, day, true, 0, out); err != nil || len(out.Rows) != 1 || out.Rows[0].Symbol != "invented19" {
		t.Fatalf("%+v %v", out, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invented unavailable", http.StatusNotFound)
	}))
	defer server.Close()
	client := cdn.New("E")
	client.HTTP = server.Client()
	client.PubMediaURL = server.URL
	app.cdn = client
	out = &weekUpdateOut{}
	if err := app.updateReferences(st, day, false, 0, out); err == nil || out.Rows[0].Status != "failed" {
		t.Fatalf("%+v %v", out, err)
	}
	data, err := runRemaining(t, st.Dir, nil, "update-week", "2026-09-28", "--dry-run", "--with-references", "--json")
	if err != nil || !strings.Contains(data, "publication_reference") {
		t.Fatalf("%s %v", data, err)
	}
}
func TestPubMediaSubtitleFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("docid") == "" && r.URL.Query().Get("pub") == "" {
			t.Fatal("missing selector")
		}
		_, _ = io.WriteString(w, `{"files":{"E":{"MP4":[{"subtitles":{"url":"https://example.invalid/invented.vtt","checksum":"abc"}}]}}}`)
	}))
	defer server.Close()
	client := cdn.New("E")
	client.HTTP = server.Client()
	client.PubMediaURL = server.URL
	a := &app{ctx: t.Context(), cdn: client, lang: "E"}
	for _, key := range []string{"pub-invented_202601_1_VIDEO", "docid-87001_1_VIDEO"} {
		u, sum, err := a.pubMediaSubtitles(key)
		if err != nil || u == "" || sum != "abc" {
			t.Fatalf("%s %s %v", u, sum, err)
		}
	}
	if u, _, err := a.pubMediaSubtitles("invented"); err != nil || u != "" {
		t.Fatalf("%s %v", u, err)
	}
}

func TestMediaClipCommand(t *testing.T) {
	st := inventedLibrary(t)
	item := cdn.MediaItem{LanguageAgnosticNaturalKey: "pub-invented_1_VIDEO", Duration: 60, Files: []cdn.MediaFile{{Label: "240p", ProgressiveDownloadURL: "https://example.invalid/invented.mp4"}}}
	if err := st.PutVideo(item.LanguageAgnosticNaturalKey, "E", "Invented clockwork film", 60, "", item); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args    []string
		wantErr bool
	}{{[]string{"--from", "1", "--to", "21"}, false}, {[]string{"--from", "bad", "--to", "21"}, true}, {[]string{"--from", "1", "--to", "bad"}, true}, {[]string{"--from", "1", "--to", "99"}, true}, {[]string{"--from", "1", "--to", "21", "--resolution", "720p"}, true}, {[]string{"--from", "1", "--to", "21", "--offline"}, true}} {
		var output bytes.Buffer
		a := &app{ctx: t.Context(), out: &output, err: io.Discard, origins: map[string]string{}, st: st, clipRun: func(_ context.Context, tool string, args ...string) ([]byte, error) {
			if tool == "ffprobe" {
				return []byte(`{"format":{"duration":"20"}}`), nil
			}
			return nil, os.WriteFile(args[len(args)-1], []byte("invented movie bytes"), 0o600)
		}}
		root := a.rootCmd()
		root.PersistentPostRunE = nil
		args := []string{"--library", st.Dir, "--language", "E", "--json", "media", "clip", "pub-invented_1_VIDEO", "--output", filepath.Join(t.TempDir(), "out.mp4")}
		args = append(args, test.args...)
		root.SetArgs(args)
		err := root.ExecuteContext(t.Context())
		if (err != nil) != test.wantErr {
			t.Fatalf("%v: %v", test.args, err)
		}
		if !test.wantErr && !strings.Contains(output.String(), `"duration_seconds": 20`) {
			t.Fatal(output.String())
		}
	}
}
