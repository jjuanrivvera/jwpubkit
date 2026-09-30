// Package cdn talks to the open JW content APIs on b.jw-cdn.org: pub-media
// (download links for publications and videos) and mediator (video metadata
// and subtitles).
package cdn

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UserAgent mimics desktop Chrome; the CDN answers plain clients too, but
// cms-imgp and wol are pickier.
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

// The two open endpoints this package reads. They are the defaults of a new
// Client rather than constants used directly, so a test can point the client at
// a local server: nothing here needs the real CDN to be exercised.
const (
	DefaultPubMediaURL = "https://b.jw-cdn.org/apis/pub-media/GETPUBMEDIALINKS"
	DefaultMediatorURL = "https://b.jw-cdn.org/apis/mediator/v1/media-items"
)

// ErrNotFound means the API answered 404: that publication, issue or video
// does not exist in that language.
var ErrNotFound = errors.New("not in the CDN")

// Client is a small HTTP client with sane timeouts.
type Client struct {
	HTTP *http.Client
	Lang string // jw.org language symbol: "E", "S", "F"…

	// PubMediaURL and MediatorURL are the endpoints to ask. New fills them
	// with the real ones; a caller overrides them to aim somewhere else.
	PubMediaURL   string
	MediatorURL   string
	CategoriesURL string
}

// New returns a client for language lang ("E").
func New(lang string) *Client {
	return &Client{
		HTTP:        &http.Client{Timeout: 10 * time.Minute},
		Lang:        lang,
		PubMediaURL: DefaultPubMediaURL,
		MediatorURL: DefaultMediatorURL,
	}
}

// pubMedia and mediator let a Client built without New still work: a zero
// value means "the real endpoint", never an empty URL.
func (c *Client) pubMedia() string {
	if c.PubMediaURL == "" {
		return DefaultPubMediaURL
	}
	return c.PubMediaURL
}

func (c *Client) mediator() string {
	if c.MediatorURL == "" {
		return DefaultMediatorURL
	}
	return c.MediatorURL
}

func (c *Client) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = c.HTTP.Do(req)
		if err == nil && resp.StatusCode < 500 {
			break
		}
		if err == nil {
			resp.Body.Close()
			err = fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)
	}
	return resp, nil
}

// GetBytes downloads a small resource into memory.
func (c *Client) GetBytes(ctx context.Context, u string) ([]byte, error) {
	resp, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	b, err := c.GetBytes(ctx, u)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("invalid response from %s: %w", u, err)
	}
	return nil
}

// PubFile is one downloadable file in a pub-media answer.
type PubFile struct {
	Title string `json:"title"`
	File  struct {
		URL              string `json:"url"`
		ModifiedDatetime string `json:"modifiedDatetime"`
		Checksum         string `json:"checksum"` // MD5 of the file
	} `json:"file"`
	Filesize    int64   `json:"filesize"`
	Label       string  `json:"label"`
	Track       int     `json:"track"`
	Pub         string  `json:"pub"`
	Docid       int     `json:"docid"`
	BookNum     int     `json:"booknum"`
	MimeType    string  `json:"mimetype"`
	Duration    float64 `json:"duration"`
	FrameHeight int     `json:"frameHeight"`
	Subtitled   bool    `json:"subtitled"`
	Subtitles   *struct {
		URL      string `json:"url"`
		Checksum string `json:"checksum"`
	} `json:"subtitles"`
}

// PubMedia is the GETPUBMEDIALINKS answer.
type PubMedia struct {
	PubName       string                          `json:"pubName"`
	Pub           string                          `json:"pub"`
	Issue         string                          `json:"issue"`
	FormattedDate string                          `json:"formattedDate"`
	Files         map[string]map[string][]PubFile `json:"files"`
}

// PubMediaQuery selects what to ask pub-media for.
type PubMediaQuery struct {
	Pub     string // "mwb", "w", "nwtsty", "jwb-125"
	Issue   string // "202609", "20130115" or ""
	Track   int
	DocID   int
	BookNum int
	Format  string // "JWPUB", "MP4"
}

// PubMedia queries the pub-media API.
func (c *Client) PubMedia(ctx context.Context, q PubMediaQuery) (*PubMedia, error) {
	v := url.Values{}
	v.Set("output", "json")
	if q.DocID != 0 {
		v.Set("docid", strconv.Itoa(q.DocID))
	} else {
		v.Set("pub", q.Pub)
	}
	if q.Issue != "" {
		v.Set("issue", q.Issue)
	}
	if q.Track != 0 {
		v.Set("track", strconv.Itoa(q.Track))
	}
	if q.BookNum != 0 {
		v.Set("booknum", strconv.Itoa(q.BookNum))
	}
	if q.Format != "" {
		v.Set("fileformat", q.Format)
	}
	v.Set("alllangs", "0")
	v.Set("langwritten", c.Lang)
	var pm PubMedia
	if err := c.getJSON(ctx, c.pubMedia()+"?"+v.Encode(), &pm); err != nil {
		return nil, err
	}
	return &pm, nil
}

// JWPUB returns the JWPUB file entry of a pub-media answer.
func (pm *PubMedia) JWPUB(lang string) (*PubFile, bool) {
	files := pm.Files[lang]["JWPUB"]
	if len(files) == 0 {
		return nil, false
	}
	return &files[0], true
}

// Download saves u to dest through a temporary file and checks the MD5 when
// one is given. It returns the MD5 it computed.
func (c *Client) Download(ctx context.Context, u, dest, wantMD5 string) (string, error) {
	resp, err := c.get(ctx, u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := md5.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if wantMD5 != "" && !strings.EqualFold(got, wantMD5) {
		os.Remove(tmp)
		return "", fmt.Errorf("MD5 mismatch for %s: expected %s, got %s", filepath.Base(dest), wantMD5, got)
	}
	return got, os.Rename(tmp, dest)
}

// FileMD5 hashes an existing file.
func FileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// MediaFile is one rendition of a video in the mediator answer.
type MediaFile struct {
	Checksum               string  `json:"checksum"`
	MimeType               string  `json:"mimetype"`
	Width                  int     `json:"width"`
	Height                 int     `json:"height"`
	Label                  string  `json:"label"`
	ProgressiveDownloadURL string  `json:"progressiveDownloadURL"`
	Filesize               int64   `json:"filesize"`
	Duration               float64 `json:"duration"`
	Subtitled              bool    `json:"subtitled"`
	Subtitles              *struct {
		URL              string `json:"url"`
		Checksum         string `json:"checksum"`
		ModifiedDatetime string `json:"modifiedDatetime"`
	} `json:"subtitles"`
}

// MediaItem is a video (or audio) in the mediator catalog.
type MediaItem struct {
	Type                       string      `json:"type"`
	FoundIn                    []string    `json:"found_in,omitempty"`
	GUID                       string      `json:"guid"`
	LanguageAgnosticNaturalKey string      `json:"languageAgnosticNaturalKey"`
	NaturalKey                 string      `json:"naturalKey"`
	Title                      string      `json:"title"`
	Description                string      `json:"description"`
	Duration                   float64     `json:"duration"`
	DurationFormatted          string      `json:"durationFormattedMinSec"`
	PrimaryCategory            string      `json:"primaryCategory"`
	FirstPublished             string      `json:"firstPublished"`
	Files                      []MediaFile `json:"files"`
}

// SubtitlesURL returns the first VTT the renditions point to.
func (m *MediaItem) SubtitlesURL() string {
	for _, f := range m.Files {
		if f.Subtitles != nil && f.Subtitles.URL != "" {
			return f.Subtitles.URL
		}
	}
	return ""
}

// MediaItem fetches one item by language-agnostic key, e.g.
// "pub-jwb-125_4_VIDEO" or "docid-702017141_1_VIDEO".
func (c *Client) MediaItem(ctx context.Context, key string) (*MediaItem, error) {
	var ans struct {
		Media []MediaItem `json:"media"`
	}
	u := fmt.Sprintf("%s/%s/%s?clientType=www", c.mediator(), c.Lang, url.PathEscape(key))
	if err := c.getJSON(ctx, u, &ans); err != nil {
		return nil, err
	}
	if len(ans.Media) == 0 {
		return nil, fmt.Errorf("%s: %w", key, ErrNotFound)
	}
	return &ans.Media[0], nil
}
