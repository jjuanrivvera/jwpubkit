// Package backup reads and edits private JW Library backups without publishing their contents.
package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Manifest struct {
	Name         string `json:"name"`
	CreationDate string `json:"creationDate"`
	Version      int    `json:"version"`
	Type         int    `json:"type"`
	Backup       struct {
		LastModified string `json:"lastModifiedDate"`
		Device       string `json:"deviceName"`
		Database     string `json:"databaseName"`
		Hash         string `json:"hash"`
		Schema       int    `json:"schemaVersion"`
	} `json:"userDataBackup"`
}

type Archive struct {
	DB       *sql.DB
	Manifest Manifest
	Files    map[string][]byte
	dir      string
}

const maxEntry = 512 << 20
const maxArchive = 1 << 30

func Open(ctx context.Context, filename string) (*Archive, error) {
	z, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	a := &Archive{Files: map[string][]byte{}}
	var total uint64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "." || f.Name == ".." || f.Name != path.Clean(f.Name) || strings.Contains(f.Name, "\\") || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") {
			return nil, fmt.Errorf("unsafe archive path %q", f.Name)
		}
		if _, exists := a.Files[f.Name]; exists {
			return nil, fmt.Errorf("duplicate archive member %q", f.Name)
		}
		if f.UncompressedSize64 > maxEntry {
			return nil, errors.New("archive entry exceeds size limit")
		}
		total += f.UncompressedSize64
		if total > maxArchive {
			return nil, errors.New("archive exceeds size limit")
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		b, e := io.ReadAll(io.LimitReader(r, maxEntry+1))
		r.Close()
		if e != nil {
			return nil, e
		}
		if len(b) > maxEntry {
			return nil, errors.New("archive entry exceeds size limit")
		}
		a.Files[f.Name] = b
	}
	if err := json.Unmarshal(a.Files["manifest.json"], &a.Manifest); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if a.Manifest.Backup.Schema != 16 || a.Manifest.Version != 1 || a.Manifest.Type != 0 || a.Manifest.Backup.Database != "userData.db" {
		return nil, errors.New("only schema 16 user-data backups are supported")
	}
	b, ok := a.Files["userData.db"]
	if !ok {
		return nil, errors.New("missing userData.db")
	}
	h := sha256.Sum256(b)
	if !strings.EqualFold(hex.EncodeToString(h[:]), a.Manifest.Backup.Hash) {
		return nil, errors.New("database hash does not match manifest")
	}
	a.dir, err = os.MkdirTemp("", "pubkit-backup-")
	if err != nil {
		return nil, err
	}
	dbpath := filepath.Join(a.dir, "userData.db")
	if err = os.WriteFile(dbpath, b, 0600); err != nil {
		a.Close()
		return nil, err
	}
	a.DB, err = sql.Open("sqlite", dbpath)
	if err != nil {
		a.Close()
		return nil, err
	}
	a.DB.SetMaxOpenConns(1)
	if _, err = a.DB.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA journal_mode=DELETE; PRAGMA busy_timeout=3000"); err != nil {
		a.Close()
		return nil, err
	}
	var version int
	if err = a.DB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 16 {
		a.Close()
		return nil, errors.New("database schema version must be 16")
	}
	if err = a.Validate(ctx); err != nil {
		a.Close()
		return nil, err
	}
	known := map[string]bool{"android_metadata": true, "LastModified": true}
	for _, table := range tableOrder {
		known[table] = true
	}
	rows, e := a.DB.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if e != nil {
		a.Close()
		return nil, e
	}
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			rows.Close()
			a.Close()
			return nil, e
		}
		if !known[name] {
			rows.Close()
			a.Close()
			return nil, fmt.Errorf("unsupported backup table %q", name)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		a.Close()
		return nil, e
	}
	return a, nil
}

func (a *Archive) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
	if a.dir != "" {
		os.RemoveAll(a.dir)
	}
}

func (a *Archive) Validate(ctx context.Context) error {
	r, err := a.DB.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	for r.Next() {
		var s string
		if err = r.Scan(&s); err != nil {
			r.Close()
			return err
		}
		if s != "ok" {
			r.Close()
			return fmt.Errorf("integrity_check: %s", s)
		}
	}
	err = r.Err()
	r.Close()
	if err != nil {
		return err
	}
	r, err = a.DB.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	bad := r.Next()
	err = r.Err()
	r.Close()
	if bad {
		return errors.New("foreign_key_check found orphan rows")
	}
	if err != nil {
		return err
	}
	for _, t := range []string{"IndependentMedia", "PlaylistItem"} {
		col := "FilePath"
		if t == "PlaylistItem" {
			col = "ThumbnailFilePath"
		}
		r, err = a.DB.QueryContext(ctx, "SELECT "+col+" FROM "+t+" WHERE "+col+" IS NOT NULL")
		if err != nil {
			return err
		}
		for r.Next() {
			var s string
			if err = r.Scan(&s); err != nil {
				r.Close()
				return err
			}
			if _, ok := a.Files[s]; !ok {
				r.Close()
				return fmt.Errorf("missing media file %q", s)
			}
		}
		err = r.Err()
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

type Inspection struct {
	Manifest    Manifest       `json:"manifest"`
	Counts      map[string]int `json:"counts"`
	Files       int            `json:"files"`
	Integrity   string         `json:"integrity_check"`
	ForeignKeys string         `json:"foreign_key_check"`
}

func (a *Archive) Inspect(ctx context.Context) (Inspection, error) {
	i := Inspection{Manifest: a.Manifest, Counts: map[string]int{}, Files: len(a.Files) - 2, Integrity: "ok", ForeignKeys: "ok"}
	for _, t := range tableOrder {
		var n int
		if err := a.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+quote(t)).Scan(&n); err != nil {
			return i, err
		}
		i.Counts[t] = n
	}
	return i, nil
}

// Save publishes only a complete, checked archive, and never replaces an existing file.
func (a *Archive) Save(ctx context.Context, filename, device string) error {
	if err := a.Validate(ctx); err != nil {
		return err
	}
	now := time.Now().UTC()
	stamp := now.Format("2006-01-02T15:04:05Z")
	if _, err := a.DB.ExecContext(ctx, "UPDATE LastModified SET LastModified=?", stamp); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(a.dir, "userData.db"))
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	a.Manifest.Name = filepath.Base(filename)
	a.Manifest.CreationDate = now.Format("2006-01-02")
	a.Manifest.Backup.LastModified = stamp
	a.Manifest.Backup.Hash = hex.EncodeToString(h[:])
	a.Manifest.Backup.Device = device
	m, err := json.Marshal(a.Manifest)
	if err != nil {
		return err
	}
	a.Files["manifest.json"] = m
	a.Files["userData.db"] = b
	f, err := os.CreateTemp(filepath.Dir(filename), ".pubkit-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	keys := make([]string, 0, len(a.Files))
	for k := range a.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, e := z.Create(k)
		if e != nil {
			z.Close()
			f.Close()
			return e
		}
		if _, e = w.Write(a.Files[k]); e != nil {
			z.Close()
			f.Close()
			return e
		}
	}
	if err = z.Close(); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), filename)
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
