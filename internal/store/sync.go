package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
)

// SyncResult reports what a sync did.
type SyncResult struct {
	Key        string        `json:"key"`
	Title      string        `json:"title"`
	File       string        `json:"file"`
	Size       int64         `json:"bytes"`
	MD5        string        `json:"md5"`
	Modified   string        `json:"modified"`
	Downloaded bool          `json:"downloaded"`
	Indexed    bool          `json:"indexed"`
	UpToDate   bool          `json:"up_to_date"`
	Download   time.Duration `json:"-"`
	Stats      *IndexStats   `json:"-"`
	Summary    string        `json:"summary,omitempty"`
	DownloadMS int64         `json:"download_ms"`
	IndexMS    int64         `json:"indexed_ms"`
}

// NormalizeIssue accepts 202609, 20260900 or 2026-09 and returns what the
// API expects (202609; semimonthly issues keep the day: 20130115).
func NormalizeIssue(issue string) string {
	issue = strings.ReplaceAll(strings.TrimSpace(issue), "-", "")
	if len(issue) == 8 && strings.HasSuffix(issue, "00") {
		issue = issue[:6]
	}
	return issue
}

// Logf receives progress lines.
type Logf func(format string, args ...any)

// Sync downloads a publication if the cached copy is missing or outdated
// (MD5 from pub-media), then indexes it. force re-downloads and re-indexes.
func (s *Store) Sync(ctx context.Context, c *cdn.Client, symbol, issue string, force bool, logf Logf) (*SyncResult, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	issue = NormalizeIssue(issue)
	pm, err := c.PubMedia(ctx, cdn.PubMediaQuery{Pub: symbol, Issue: issue, Format: "JWPUB"})
	if errors.Is(err, cdn.ErrNotFound) {
		what := symbol
		if issue != "" {
			what += " " + issue
		}
		return nil, fmt.Errorf("pub-media no tiene %s en JWPUB para el idioma %s (HTTP 404)", what, c.Lang)
	}
	if err != nil {
		return nil, err
	}
	f, ok := pm.JWPUB(c.Lang)
	if !ok {
		return nil, fmt.Errorf("pub-media returned no JWPUB for %s %s", symbol, issue)
	}
	info := PubInfo{Symbol: symbol, Issue: issue, Lang: c.Lang, MD5: f.File.Checksum, Size: f.Filesize, Modified: f.File.ModifiedDatetime}
	info.File = filepath.Join(s.PubsDir(), path.Base(f.File.URL))
	res := &SyncResult{Key: info.Key(), Title: pm.PubName, File: info.File, Size: f.Filesize, MD5: f.File.Checksum, Modified: f.File.ModifiedDatetime}
	if pm.FormattedDate != "" {
		res.Title += " (" + strings.ReplaceAll(pm.FormattedDate, "&nbsp;", " ") + ")"
	}

	var dbMD5 string
	err = s.DB.QueryRow(`SELECT md5 FROM pub WHERE key=?`, info.Key()).Scan(&dbMD5)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	st, statErr := os.Stat(info.File)
	cached := statErr == nil && st.Size() == f.Filesize
	if cached && !force {
		if dbMD5 != "" && strings.EqualFold(dbMD5, f.File.Checksum) {
			res.UpToDate = true
			return res, nil
		}
		// A file of the right size but never indexed (or indexed from another
		// version) is checked before trusting it.
		if sum, err := cdn.FileMD5(info.File); err != nil || !strings.EqualFold(sum, f.File.Checksum) {
			cached = false
		}
	}
	if !cached || force {
		logf("downloading %s (%.1f MB)…", path.Base(f.File.URL), float64(f.Filesize)/1e6)
		t := time.Now()
		if _, err := c.Download(ctx, f.File.URL, info.File, f.File.Checksum); err != nil {
			return nil, err
		}
		res.Download = time.Since(t)
		res.DownloadMS = res.Download.Milliseconds()
		res.Downloaded = true
	}
	logf("decrypting and indexing %s…", info.Key())
	stats, err := s.Index(info)
	if err != nil {
		return nil, err
	}
	res.Indexed = true
	res.Stats = stats
	res.IndexMS = stats.Elapsed.Milliseconds()
	res.Summary = stats.String()
	return res, nil
}

// IndexLocal indexes a .jwpub the user already has (for offline use).
func (s *Store) IndexLocal(file, symbol, issue, lang string) (*IndexStats, error) {
	st, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	sum, err := cdn.FileMD5(file)
	if err != nil {
		return nil, err
	}
	// filepath.Base drops whatever path the name carries, so the copy cannot land
	// outside the publications directory.
	dest := filepath.Join(s.PubsDir(), filepath.Base(file))
	if abs, _ := filepath.Abs(file); abs != dest {
		if err := copyFile(file, dest); err != nil {
			return nil, err
		}
	}
	return s.Index(PubInfo{Symbol: symbol, Issue: NormalizeIssue(issue), Lang: lang, File: dest, MD5: sum, Size: st.Size(),
		Modified: st.ModTime().Format("2006-01-02 15:04:05")})
}

// copyFile streams: a .jwpub can run to hundreds of megabytes and there is no
// reason to hold one in memory.
func copyFile(src, dest string) error {
	// #nosec G304,G703 -- src is exactly the file the user named in --file; reading
	// it is what the command was asked to do.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
