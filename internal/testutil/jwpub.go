// Package testutil builds small synthetic JWPUB files for tests, encrypted
// with the real scheme, so tests need neither the network nor copyrighted
// publication content.
package testutil

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/jjuanrivvera/jwpubkit/internal/jwpub"
)

// Doc is a document of the synthetic publication.
type Doc struct {
	ID      int // local DocumentId
	MepsID  int
	Class   int
	Title   string
	Context string
	HTML    string
}

// Pub describes the synthetic publication.
type Pub struct {
	Symbol    string // Publication.Symbol, e.g. "mwb26"
	Undated   string // "mwb"
	Year      int
	IssueTag  int
	Title     string
	Docs      []Doc
	Dated     [][4]any // DocumentId, first, last, link
	Extracts  []Extract
	Media     []Media
	Verses    map[int]string // BibleVerseId -> verse html (makes it a Bible)
	ExtraSQL  []string
	ImageName string
	ImageData []byte
}

// Extract is a DocumentExtract + Extract row.
type Extract struct {
	DocID     int
	ExtractID int
	Link      string
	Caption   string
	HTML      string
	RefDocID  int
	// RefClass is the class the publication gives the referenced document: 31 a
	// song, 13 a book chapter, 40 a magazine article. It is not translated,
	// unlike RefSymbol, so it is what the parser can key on. Zero means 40.
	RefClass   int
	RefSymbol  string
	RefUndated string
	RefIssue   int
	BeginPID   int
	Sort       int
}

// Media is a DocumentMultimedia + Multimedia row.
type Media struct {
	DocID    int
	ID       int
	File     string
	Label    string
	Caption  string
	BeginPID int
	Width    int
	Height   int
}

// Build writes the JWPUB into dir and returns its path.
func Build(t testing.TB, dir string, p Pub) string {
	t.Helper()
	card := jwpub.Card{MepsLanguageIndex: 1, Symbol: p.Symbol, Year: p.Year, IssueTagNumber: p.IssueTag}
	ci := jwpub.NewCipher(card)
	enc := func(s string) []byte {
		b, err := ci.Encrypt([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	dbPath := filepath.Join(t.TempDir(), p.Symbol+".db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE Publication(PublicationId INTEGER, Title TEXT, ShortTitle TEXT, Symbol TEXT, UndatedSymbol TEXT, Year INTEGER,
			IssueTagNumber INTEGER, MepsLanguageIndex INTEGER, PublicationType TEXT, PublicationCategorySymbol TEXT,
			FirstDatedTextDateOffset INTEGER, LastDatedTextDateOffset INTEGER)`,
		`CREATE TABLE Document(DocumentId INTEGER, MepsDocumentId INTEGER, Class INTEGER, Type INTEGER, SectionNumber INTEGER,
			ChapterNumber INTEGER, Title TEXT, TocTitle TEXT, ContextTitle TEXT, FeatureTitle TEXT, FirstPageNumber INTEGER,
			LastPageNumber INTEGER, Content BLOB)`,
		`CREATE TABLE DatedText(DatedTextId INTEGER PRIMARY KEY, DocumentId INTEGER, Link TEXT, FirstDateOffset INTEGER,
			LastDateOffset INTEGER, Caption TEXT)`,
		`CREATE TABLE Extract(ExtractId INTEGER, Link TEXT, Caption TEXT, Content BLOB, RefPublicationId INTEGER,
			RefMepsDocumentId INTEGER, RefMepsDocumentClass INTEGER, RefBeginParagraphOrdinal INTEGER, RefEndParagraphOrdinal INTEGER)`,
		`CREATE TABLE DocumentExtract(DocumentExtractId INTEGER PRIMARY KEY, DocumentId INTEGER, ExtractId INTEGER,
			BeginParagraphOrdinal INTEGER, EndParagraphOrdinal INTEGER, SortPosition INTEGER)`,
		`CREATE TABLE RefPublication(RefPublicationId INTEGER, Symbol TEXT, UndatedSymbol TEXT, IssueTagNumber TEXT, ShortTitle TEXT)`,
		`CREATE TABLE Multimedia(MultimediaId INTEGER, DataType INTEGER, MimeType TEXT, Width INTEGER, Height INTEGER, Label TEXT,
			Caption TEXT, CategoryType INTEGER, FilePath TEXT, KeySymbol TEXT, Track INTEGER, MepsDocumentId INTEGER, IssueTagNumber INTEGER)`,
		`CREATE TABLE DocumentMultimedia(DocumentMultimediaId INTEGER PRIMARY KEY, DocumentId INTEGER, MultimediaId INTEGER,
			BeginParagraphOrdinal INTEGER, EndParagraphOrdinal INTEGER)`,
	}
	if p.Verses != nil {
		stmts = append(stmts,
			`CREATE TABLE BibleVerse(BibleVerseId INTEGER, Label TEXT, Content BLOB)`,
			`CREATE TABLE BibleChapter(BibleChapterId INTEGER, BookNumber INTEGER, ChapterNumber INTEGER, Content BLOB)`)
	}
	for _, q := range append(stmts, p.ExtraSQL...) {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO Publication VALUES(1,?,?,?,?,?,?,1,'Test','t',0,0)`, p.Title, p.Title, p.Symbol, p.Undated, p.Year, p.IssueTag)
	for _, d := range p.Docs {
		mustExec(`INSERT INTO Document VALUES(?,?,?,0,0,0,?,?,?,'',0,0,?)`, d.ID, d.MepsID, d.Class, d.Title, d.Title, d.Context, enc(d.HTML))
	}
	for _, dt := range p.Dated {
		mustExec(`INSERT INTO DatedText(DocumentId, FirstDateOffset, LastDateOffset, Link, Caption) VALUES(?,?,?,?,'')`, dt[0], dt[1], dt[2], dt[3])
	}
	for i, e := range p.Extracts {
		mustExec(`INSERT INTO RefPublication VALUES(?,?,?,?,?)`, i+1, e.RefSymbol, e.RefUndated, e.RefIssue, e.RefSymbol)
		class := e.RefClass
		if class == 0 {
			class = 40
		}
		mustExec(`INSERT INTO Extract VALUES(?,?,?,?,?,?,?,0,0)`, e.ExtractID, e.Link, e.Caption, enc(e.HTML), i+1, e.RefDocID, class)
		mustExec(`INSERT INTO DocumentExtract(DocumentId, ExtractId, BeginParagraphOrdinal, EndParagraphOrdinal, SortPosition) VALUES(?,?,?,?,?)`,
			e.DocID, e.ExtractID, e.BeginPID, e.BeginPID, e.Sort)
	}
	for _, m := range p.Media {
		mustExec(`INSERT INTO Multimedia VALUES(?,0,'image/jpeg',?,?,?,?,8,?,NULL,NULL,NULL,0)`, m.ID, m.Width, m.Height, m.Label, m.Caption, m.File)
		mustExec(`INSERT INTO DocumentMultimedia(DocumentId, MultimediaId, BeginParagraphOrdinal, EndParagraphOrdinal) VALUES(?,?,?,?)`,
			m.DocID, m.ID, m.BeginPID, m.BeginPID)
	}
	for id, h := range p.Verses {
		mustExec(`INSERT INTO BibleVerse VALUES(?,'',?)`, id, enc(h))
	}
	if p.Verses != nil {
		mustExec(`INSERT INTO BibleChapter VALUES(1,0,0,?)`, enc("<p></p>"))
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	var inner bytes.Buffer
	zw := zip.NewWriter(&inner)
	add := func(name string, data []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	add(p.Symbol+".db", dbBytes)
	if p.ImageName != "" {
		add(p.ImageName, p.ImageData)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	manifest, _ := json.Marshal(map[string]any{
		"name": p.Symbol + ".jwpub", "publication": map[string]any{"fileName": p.Symbol + ".db", "symbol": p.Symbol, "year": p.Year, "language": 1},
	})
	var outer bytes.Buffer
	ow := zip.NewWriter(&outer)
	mw, _ := ow.Create("manifest.json")
	mw.Write(manifest)
	// JW stores "contents" uncompressed; keep it that way to exercise the
	// in-place reader.
	cw, err := ow.CreateHeader(&zip.FileHeader{Name: "contents", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	cw.Write(inner.Bytes())
	if err := ow.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, p.Symbol+".jwpub")
	if err := os.WriteFile(path, outer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
