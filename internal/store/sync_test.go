package store

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// syncFixture stands a pub-media API up in front of a synthetic publication: the
// same bytes the real CDN would hand over, built and encrypted here, never taken
// from a publication. It reports how many times the file was actually fetched,
// which is the only way to tell a cache hit from a silent re-download.
func syncFixture(t *testing.T) (*Store, *cdn.Client, *int32) {
	t.Helper()
	pub := testutil.Pub{
		Symbol: "mwb26.01", Undated: "mwb", Year: 2026, IssueTag: 20260100,
		Title: "Invented Workbook January 2026",
		Docs: []testutil.Doc{
			{ID: 1, MepsID: 2026101, Class: 106, Title: "Week of January 5",
				HTML: `<p id="p1" data-pid="1">An invented paragraph of the first week.</p>`},
			{ID: 2, MepsID: 2026102, Class: 106, Title: "Week of January 12",
				HTML: `<p id="p1" data-pid="1">An invented paragraph of the second week.</p>`},
		},
	}
	file := testutil.Build(t, t.TempDir(), pub)
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(body)
	checksum := hex.EncodeToString(sum[:])

	var fetches int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/mwb26.jwpub", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&fetches, 1)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/pub-media", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pub") != "mwb" {
			http.NotFound(w, r)
			return
		}
		f := cdn.PubFile{Title: "Invented Workbook January 2026", Filesize: int64(len(body))}
		f.File.URL = srv.URL + "/mwb26.jwpub"
		f.File.Checksum = checksum
		f.File.ModifiedDatetime = "2026-01-01T00:00:00Z"
		_ = json.NewEncoder(w).Encode(cdn.PubMedia{
			PubName: "Invented Workbook", Pub: "mwb", Issue: "202601",
			FormattedDate: "January&nbsp;2026",
			Files:         map[string]map[string][]cdn.PubFile{"E": {"JWPUB": {f}}},
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s, err := Open(t.TempDir(), "E")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	c := cdn.New("E")
	c.HTTP = srv.Client()
	c.PubMediaURL = srv.URL + "/pub-media"
	return s, c, &fetches
}

// The whole path in one go: ask pub-media, download, verify the checksum,
// decrypt and index. Everything else in this package assumes it worked.
func TestSyncDownloadsDecryptsAndIndexes(t *testing.T) {
	s, c, fetches := syncFixture(t)

	res, err := s.Sync(t.Context(), c, "mwb", "202601", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Downloaded || !res.Indexed || res.UpToDate {
		t.Errorf("first sync = downloaded %v, indexed %v, up-to-date %v", res.Downloaded, res.Indexed, res.UpToDate)
	}
	// The formatted date belongs in the title, with the non-breaking space
	// turned into one a terminal can print.
	if !strings.Contains(res.Title, "January 2026") || strings.Contains(res.Title, "&nbsp;") {
		t.Errorf("Title = %q", res.Title)
	}
	if res.Stats == nil || res.Summary == "" {
		t.Error("a sync that indexed should say what it indexed")
	}
	if _, err := os.Stat(res.File); err != nil {
		t.Errorf("the publication should be on disk at %s: %v", res.File, err)
	}
	if filepath.Dir(res.File) != s.PubsDir() {
		t.Errorf("the publication landed outside the library: %s", res.File)
	}

	// And it is queryable, which is the point of indexing it.
	pub, err := s.PubByKey(res.Key)
	if err != nil || pub == nil {
		t.Fatalf("PubByKey(%q) = %+v, %v", res.Key, pub, err)
	}
	// PubByKey answers about the publication, not its contents (only Pubs()
	// counts documents), so the documents are checked where they landed.
	docs, err := s.Pubs()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Docs != 2 {
		t.Errorf("the library should hold one publication with its two documents: %+v", docs)
	}
	if got, err := s.Summaries([]int{2026101, 2026102}); err != nil || len(got) != 2 {
		t.Errorf("both documents should be queryable by MEPS id: %+v, %v", got, err)
	}
	if n := atomic.LoadInt32(fetches); n != 1 {
		t.Errorf("the file was fetched %d times, want 1", n)
	}
}

// Syncing again must not download the same bytes twice: the MD5 recorded in the
// library is what makes "already up to date" trustworthy.
func TestSyncSecondTimeIsAnAnswer(t *testing.T) {
	s, c, fetches := syncFixture(t)
	if _, err := s.Sync(t.Context(), c, "mwb", "202601", false, nil); err != nil {
		t.Fatal(err)
	}

	res, err := s.Sync(t.Context(), c, "mwb", "202601", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpToDate || res.Downloaded || res.Indexed {
		t.Errorf("second sync = up-to-date %v, downloaded %v, indexed %v", res.UpToDate, res.Downloaded, res.Indexed)
	}
	if n := atomic.LoadInt32(fetches); n != 1 {
		t.Errorf("the file was fetched %d times; the second sync should not have downloaded", n)
	}

	// --force is what makes it ask again anyway.
	forced, err := s.Sync(t.Context(), c, "mwb", "202601", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !forced.Downloaded || !forced.Indexed {
		t.Errorf("forced sync = downloaded %v, indexed %v", forced.Downloaded, forced.Indexed)
	}
	if n := atomic.LoadInt32(fetches); n != 2 {
		t.Errorf("after --force the file was fetched %d times, want 2", n)
	}
}

// A publication the CDN does not have must say so in terms the reader can act
// on — which publication, which language — instead of surfacing a bare 404.
func TestSyncNamesWhatItCouldNotFind(t *testing.T) {
	s, c, _ := syncFixture(t)

	_, err := s.Sync(t.Context(), c, "nope", "202601", false, nil)
	if err == nil {
		t.Fatal("a publication that does not exist should fail")
	}
	for _, want := range []string{"nope", "202601", "language E"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should name %q: %v", want, err)
		}
	}
}

// Progress goes to the caller's logger, not to stdout: the JSON output of the
// CLI has to stay parseable while a sync narrates itself on stderr.
func TestSyncReportsProgressThroughTheLogger(t *testing.T) {
	s, c, _ := syncFixture(t)

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, format) }
	if _, err := s.Sync(t.Context(), c, "mwb", "202601", false, logf); err != nil {
		t.Fatal(err)
	}
	if len(lines) < 2 {
		t.Errorf("a first sync should report downloading and indexing, got %v", lines)
	}

	// A nil logger is not a crash: most callers pass one.
	if _, err := s.Sync(t.Context(), c, "mwb", "202601", true, nil); err != nil {
		t.Fatal(err)
	}
}
