package jwpub

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver keeps the binary static
)

// Manifest is manifest.json at the root of a JWPUB.
type Manifest struct {
	Name          string `json:"name"`
	Hash          string `json:"hash"`
	Timestamp     string `json:"timestamp"`
	ContentFormat string `json:"contentFormat"`
	Publication   struct {
		FileName              string `json:"fileName"`
		Type                  int    `json:"type"`
		Title                 string `json:"title"`
		ShortTitle            string `json:"shortTitle"`
		DisplayTitle          string `json:"displayTitle"`
		ReferenceTitle        string `json:"referenceTitle"`
		UndatedReferenceTitle string `json:"undatedReferenceTitle"`
		Symbol                string `json:"symbol"`
		UndatedSymbol         string `json:"undatedSymbol"`
		Language              int    `json:"language"`
		Year                  int    `json:"year"`
		IssueID               int    `json:"issueId"`
		IssueNumber           int    `json:"issueNumber"`
		PublicationType       string `json:"publicationType"`
		SchemaVersion         int    `json:"schemaVersion"`
	} `json:"publication"`
}

// File is an opened JWPUB: the outer zip, the inner "contents" zip (database
// and images) and the database extracted to a temporary file.
type File struct {
	Path     string
	Manifest Manifest
	Card     Card
	Cipher   Cipher
	DB       *sql.DB

	f        *os.File
	contents *zip.Reader
	tmpDir   string
	cols     map[string]map[string]bool
}

// OpenContents opens only the inner zip, which is enough to read images.
func OpenContents(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	jf := &File{Path: path, f: f}
	if err := jf.openZips(); err != nil {
		f.Close()
		return nil, err
	}
	return jf, nil
}

// Open opens a JWPUB, extracts its database to a temporary directory and
// derives the content key from the Publication table.
func Open(path string) (*File, error) { return OpenIn(path, "") }

// OpenIn is Open with the temporary database under tmpDir; the Insight
// database alone is 40 MB, too much for a small tmpfs.
func OpenIn(path, tmpDir string) (*File, error) {
	jf, err := OpenContents(path)
	if err != nil {
		return nil, err
	}
	if err := jf.openDB(tmpDir); err != nil {
		jf.Close()
		return nil, err
	}
	return jf, nil
}

func (jf *File) openZips() error {
	st, err := jf.f.Stat()
	if err != nil {
		return err
	}
	outer, err := zip.NewReader(jf.f, st.Size())
	if err != nil {
		return fmt.Errorf("%s no es un JWPUB (zip): %w", filepath.Base(jf.Path), err)
	}
	var contents *zip.File
	for _, zf := range outer.File {
		switch zf.Name {
		case "manifest.json":
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			err = json.NewDecoder(rc).Decode(&jf.Manifest)
			rc.Close()
			if err != nil {
				return fmt.Errorf("manifest.json: %w", err)
			}
		case "contents":
			contents = zf
		}
	}
	if contents == nil {
		return errors.New("el JWPUB no trae el archivo contents")
	}
	// "contents" is normally stored uncompressed, so the inner zip can be read
	// in place instead of copying a 300 MB entry into memory.
	var ra io.ReaderAt
	size := int64(contents.UncompressedSize64)
	if contents.Method == zip.Store {
		off, err := contents.DataOffset()
		if err != nil {
			return err
		}
		ra = io.NewSectionReader(jf.f, off, size)
	} else {
		rc, err := contents.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		ra = bytes.NewReader(b)
	}
	inner, err := zip.NewReader(ra, size)
	if err != nil {
		return fmt.Errorf("contents: %w", err)
	}
	jf.contents = inner
	return nil
}

func (jf *File) openDB(tmpDir string) error {
	var dbEntry *zip.File
	for _, zf := range jf.contents.File {
		if strings.HasSuffix(zf.Name, ".db") && (dbEntry == nil || zf.Name == jf.Manifest.Publication.FileName) {
			dbEntry = zf
		}
	}
	if dbEntry == nil {
		return errors.New("el JWPUB no trae base de datos")
	}
	if tmpDir != "" {
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.MkdirTemp(tmpDir, "jwlib-")
	if err != nil {
		return err
	}
	jf.tmpDir = tmp
	dbPath := filepath.Join(tmp, filepath.Base(dbEntry.Name))
	if err := extract(dbEntry, dbPath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		return err
	}
	jf.DB = db
	var c Card
	var issue sql.NullInt64
	err = db.QueryRow(`SELECT MepsLanguageIndex, Symbol, Year, IssueTagNumber FROM Publication LIMIT 1`).
		Scan(&c.MepsLanguageIndex, &c.Symbol, &c.Year, &issue)
	if err != nil {
		return fmt.Errorf("tabla Publication: %w", err)
	}
	c.IssueTagNumber = int(issue.Int64)
	jf.Card = c
	jf.Cipher = NewCipher(c)
	return nil
}

func extract(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Close releases the database and removes the temporary copy.
func (jf *File) Close() error {
	var errs []error
	if jf.DB != nil {
		errs = append(errs, jf.DB.Close())
	}
	if jf.f != nil {
		errs = append(errs, jf.f.Close())
	}
	if jf.tmpDir != "" {
		errs = append(errs, os.RemoveAll(jf.tmpDir))
	}
	return errors.Join(errs...)
}

// ReadFile returns a file (usually an image) from the inner zip.
func (jf *File) ReadFile(name string) ([]byte, error) {
	for _, zf := range jf.contents.File {
		if zf.Name == name {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s no está dentro de %s", name, filepath.Base(jf.Path))
}

// HasFile reports whether the inner zip holds name.
func (jf *File) HasFile(name string) bool {
	for _, zf := range jf.contents.File {
		if zf.Name == name {
			return true
		}
	}
	return false
}

// Decrypt decrypts a content blob with this publication's key.
func (jf *File) Decrypt(blob []byte) (string, error) {
	return jf.Cipher.DecryptString(blob)
}

// HasTable reports whether the publication database has table.
func (jf *File) HasTable(table string) bool {
	return len(jf.Columns(table)) > 0
}

// Columns lists the columns of table. Older publications (a 2006 Watchtower)
// use earlier schema versions, so queries check before selecting.
func (jf *File) Columns(table string) map[string]bool {
	if jf.cols == nil {
		jf.cols = map[string]map[string]bool{}
	}
	if c, ok := jf.cols[table]; ok {
		return c
	}
	c := map[string]bool{}
	rows, err := jf.DB.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err == nil {
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk) == nil {
				c[name] = true
			}
		}
		rows.Close()
	}
	jf.cols[table] = c
	return c
}

// Col returns "table.col" when the column exists and "NULL" otherwise, for
// building SELECTs that work across schema versions.
func (jf *File) Col(table, col string) string {
	if jf.Columns(table)[col] {
		return table + "." + col
	}
	return "NULL"
}
