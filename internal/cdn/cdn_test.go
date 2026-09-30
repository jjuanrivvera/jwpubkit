package cdn

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// clientFor returns a client aimed at srv, with both endpoints redirected and
// no retry patience: a test must never wait on the real backoff.
func clientFor(srv *httptest.Server) *Client {
	c := New("E")
	c.HTTP = srv.Client()
	c.PubMediaURL = srv.URL + "/pub-media"
	c.MediatorURL = srv.URL + "/mediator"
	return c
}

// A zero-value Client is still expected to reach the real CDN: code elsewhere
// builds one by hand, and an empty URL would silently fetch nothing.
func TestZeroClientKeepsTheRealEndpoints(t *testing.T) {
	var c Client
	if got := c.pubMedia(); got != DefaultPubMediaURL {
		t.Errorf("pub-media endpoint of a zero Client = %q, want the default", got)
	}
	if got := c.mediator(); got != DefaultMediatorURL {
		t.Errorf("mediator endpoint of a zero Client = %q, want the default", got)
	}
	if New("S").Lang != "S" {
		t.Error("New should keep the language it was given")
	}
}

func TestPubMedia_SendsTheQueryTheAPIExpects(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("User-Agent = %q, want the desktop one", r.Header.Get("User-Agent"))
		}
		_ = json.NewEncoder(w).Encode(PubMedia{PubName: "Invented Workbook", Pub: "mwb", Issue: "202601"})
	}))
	defer srv.Close()

	pm, err := clientFor(srv).PubMedia(t.Context(), PubMediaQuery{Pub: "mwb", Issue: "202601", Format: "JWPUB"})
	if err != nil {
		t.Fatalf("PubMedia: %v", err)
	}
	if pm.PubName != "Invented Workbook" {
		t.Errorf("PubName = %q", pm.PubName)
	}
	for k, want := range map[string]string{
		"output": "json", "pub": "mwb", "issue": "202601",
		"fileformat": "JWPUB", "alllangs": "0", "langwritten": "E",
	} {
		if got.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, got.Get(k), want)
		}
	}
}

// A docid query must send docid and NOT pub: the API treats them as different
// lookups, and sending both makes it answer for the publication, not the document.
func TestPubMedia_DocIDReplacesThePublication(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := clientFor(srv).PubMedia(t.Context(), PubMediaQuery{Pub: "mwb", DocID: 1102023801, Track: 4, BookNum: 43}); err != nil {
		t.Fatalf("PubMedia: %v", err)
	}
	if got.Get("docid") != "1102023801" {
		t.Errorf("docid = %q", got.Get("docid"))
	}
	if got.Has("pub") {
		t.Errorf("pub should not be sent alongside docid, got %q", got.Get("pub"))
	}
	if got.Get("track") != "4" || got.Get("booknum") != "43" {
		t.Errorf("track/booknum = %q/%q", got.Get("track"), got.Get("booknum"))
	}
}

func TestPubMedia_404IsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := clientFor(srv).PubMedia(t.Context(), PubMediaQuery{Pub: "nope"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPubMedia_BadJSONNamesTheURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()

	_, err := clientFor(srv).PubMedia(t.Context(), PubMediaQuery{Pub: "mwb"})
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Fatalf("want an invalid-response error naming the URL, got %v", err)
	}
}

// 500 is the CDN having a bad minute, so it is retried; 403 is an answer, so it
// is not. Retrying a refusal only makes the caller wait to be refused again.
func TestGet_RetriesServerErrorsButNotRefusals(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("second time lucky"))
	}))
	defer srv.Close()
	c := clientFor(srv)

	b, err := c.GetBytes(t.Context(), srv.URL+"/flaky")
	if err != nil {
		t.Fatalf("GetBytes: %v", err)
	}
	if string(b) != "second time lucky" || atomic.LoadInt32(&hits) != 2 {
		t.Errorf("got %q after %d attempts", b, hits)
	}

	refuser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer refuser.Close()
	atomic.StoreInt32(&hits, 0)
	_, err = clientFor(refuser).GetBytes(t.Context(), refuser.URL+"/no")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("want an HTTP 403 error, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("a refusal was asked %d times; it should be asked once", n)
	}
}

// A cancelled context must abandon the wait between retries rather than sleep
// it out: that wait is where a Ctrl-C during a sync actually lands.
func TestGet_CancelledDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if _, err := clientFor(srv).GetBytes(ctx, srv.URL+"/slow"); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("waited %v after cancellation; it should give up at once", elapsed)
	}
}

func TestJWPUB_PicksTheFileForTheLanguage(t *testing.T) {
	pm := &PubMedia{Files: map[string]map[string][]PubFile{
		"E": {"JWPUB": {{Title: "Invented Workbook", Filesize: 4096}}},
		"S": {"MP4": {{Title: "Invented Video"}}},
	}}
	f, ok := pm.JWPUB("E")
	if !ok || f.Filesize != 4096 {
		t.Fatalf("JWPUB(E) = %+v, %v", f, ok)
	}
	if _, ok := pm.JWPUB("S"); ok {
		t.Error("a language with no JWPUB should report none, not an MP4")
	}
	if _, ok := pm.JWPUB("F"); ok {
		t.Error("an absent language should report none")
	}
}

func TestDownload_WritesTheFileAndChecksTheMD5(t *testing.T) {
	body := []byte("invented publication bytes")
	sum := md5.Sum(body)
	want := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	c := clientFor(srv)

	dest := filepath.Join(t.TempDir(), "nested", "pub.jwpub")
	got, err := c.Download(t.Context(), srv.URL+"/f", dest, want)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got != want {
		t.Errorf("md5 = %s, want %s", got, want)
	}
	on, err := os.ReadFile(dest)
	if err != nil || string(on) != string(body) {
		t.Fatalf("file on disk = %q, err %v", on, err)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Error("the temporary file should be gone once the download succeeded")
	}

	// A wrong checksum must leave nothing behind: a half-written publication
	// that looks complete is worse than no publication.
	bad := filepath.Join(t.TempDir(), "pub.jwpub")
	if _, err := c.Download(t.Context(), srv.URL+"/f", bad, "00000000000000000000000000000000"); err == nil {
		t.Fatal("a checksum mismatch should fail")
	} else if !strings.Contains(err.Error(), "MD5 mismatch") {
		t.Errorf("error should name the mismatch: %v", err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Error("the destination should not exist after a mismatch")
	}
	if _, err := os.Stat(bad + ".part"); !os.IsNotExist(err) {
		t.Error("the partial file should be removed after a mismatch")
	}

	// No checksum given means the caller has nothing to compare, not that the
	// download is skipped.
	plain := filepath.Join(t.TempDir(), "pub.jwpub")
	if got, err := c.Download(t.Context(), srv.URL+"/f", plain, ""); err != nil || got != want {
		t.Errorf("Download with no wanted md5 = %s, %v", got, err)
	}
}

func TestDownload_PropagatesTheHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "pub.jwpub")
	if _, err := clientFor(srv).Download(t.Context(), srv.URL+"/gone", dest, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestFileMD5(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	body := []byte("invented bytes")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(body)
	got, err := FileMD5(p)
	if err != nil || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("FileMD5 = %s, %v", got, err)
	}
	if _, err := FileMD5(filepath.Join(dir, "missing")); err == nil {
		t.Error("hashing a file that is not there should fail")
	}
}

func TestMediaItem(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if strings.Contains(r.URL.Path, "empty") {
			_, _ = w.Write([]byte(`{"media":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"media":[{"title":"Invented Video","naturalKey":"pub-inv-1_1_VIDEO",
			"files":[{"label":"240p","subtitled":false},
			         {"label":"720p","subtitled":true,"subtitles":{"url":"https://example.invalid/inv.vtt"}}]}]}`))
	}))
	defer srv.Close()
	c := clientFor(srv)

	item, err := c.MediaItem(t.Context(), "pub-inv-1_1_VIDEO")
	if err != nil {
		t.Fatalf("MediaItem: %v", err)
	}
	if item.Title != "Invented Video" {
		t.Errorf("Title = %q", item.Title)
	}
	if want := "/mediator/E/pub-inv-1_1_VIDEO"; path != want {
		t.Errorf("asked %q, want %q — the language belongs in the path", path, want)
	}
	if got := item.SubtitlesURL(); got != "https://example.invalid/inv.vtt" {
		t.Errorf("SubtitlesURL = %q; it should skip the rendition without subtitles", got)
	}

	// An answer with an empty media list is a miss, not a success with no data.
	if _, err := c.MediaItem(t.Context(), "empty"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for an empty answer, got %v", err)
	}
}

func TestSubtitlesURL_NoneAnywhere(t *testing.T) {
	m := &MediaItem{Files: []MediaFile{{Label: "240p"}, {Label: "720p", Subtitled: true}}}
	if got := m.SubtitlesURL(); got != "" {
		t.Errorf("SubtitlesURL = %q, want empty when no rendition carries one", got)
	}
}
