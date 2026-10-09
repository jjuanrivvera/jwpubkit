package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name, stamp string, seed func(*sql.DB), files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "userData.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic fixture construction needs no crash durability; saved archives are still validated.
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(t.Context(), "PRAGMA journal_mode=MEMORY; PRAGMA synchronous=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(t.Context(), testSchema); err != nil {
		t.Fatal(err)
	}
	if seed != nil {
		seed(db)
	}
	if _, err = db.ExecContext(t.Context(), "UPDATE LastModified SET LastModified=?", stamp); err != nil {
		t.Fatal(err)
	}
	db.Close()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	m := Manifest{Name: name, Version: 1, Type: 0}
	m.Backup.Schema = 16
	m.Backup.Database = "userData.db"
	m.Backup.Device = name
	m.Backup.LastModified = stamp
	m.Backup.Hash = hex.EncodeToString(h[:])
	mb, _ := json.Marshal(m)
	if files == nil {
		files = map[string][]byte{}
	}
	files["userData.db"] = b
	files["manifest.json"] = mb
	out := filepath.Join(dir, name+".jwlibrary")
	writeZip(t, out, files)
	return out
}

func writeZip(t *testing.T, p string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, b := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}
func execTest(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), q, args...); err != nil {
		t.Fatal(err)
	}
}

func seedRows(t *testing.T, db *sql.DB, offset int, variant string) {
	execTest(t, db, `INSERT INTO Location(LocationId,DocumentId,KeySymbol,MepsLanguage,Type) VALUES(?,123456,'fictional',0,0)`, offset+1)
	execTest(t, db, `INSERT INTO UserMark VALUES(?,1,?,0,'shared-mark',1)`, offset+2, offset+1)
	execTest(t, db, `INSERT INTO BlockRange VALUES(?,1,4,0,4,?)`, offset+3, offset+2)
	execTest(t, db, `INSERT INTO Note(NoteId,Guid,UserMarkId,LocationId,Title,Content,BlockType,BlockIdentifier) VALUES(?,'shared-note',?,?,'Invented note',?,1,4)`, offset+4, offset+2, offset+1, variant)
	execTest(t, db, `INSERT INTO Tag VALUES(?,0,'Invented tag')`, offset+5)
	execTest(t, db, `INSERT INTO TagMap(TagMapId,NoteId,TagId,Position) VALUES(?,?,?,0)`, offset+6, offset+4, offset+5)
	execTest(t, db, `INSERT INTO InputField VALUES(?,'tt11',?)`, offset+1, variant)
	execTest(t, db, `INSERT INTO Bookmark VALUES(?,?,?,0,'Invented bookmark',?,1,4)`, offset+7, offset+1, offset+1, variant)
}

func TestMergePriorityAndRemapping(t *testing.T) {
	a := fixture(t, "alpha", "2026-01-01T00:00:00Z", func(db *sql.DB) { seedRows(t, db, 0, "A") }, nil)
	b := fixture(t, "beta", "2026-02-01T00:00:00Z", func(db *sql.DB) {
		seedRows(t, db, 100, "B")
		execTest(t, db, `INSERT INTO UserMark VALUES(120,2,101,0,'overlapping',1); INSERT INTO BlockRange VALUES(121,1,4,2,8,120); INSERT INTO Note(NoteId,Guid,UserMarkId,LocationId,Title,Content) VALUES(122,'only-beta',120,101,'Invented extra','Extra'); INSERT INTO TagMap(TagMapId,NoteId,TagId,Position) VALUES(123,122,105,1)`)
	}, nil)
	c := fixture(t, "gamma", "2026-03-01T00:00:00Z", func(db *sql.DB) { seedRows(t, db, 200, "C") }, nil)
	for _, tc := range []struct {
		name, prefer, want string
		prefs              map[string]string
	}{{"first", "", "A", nil}, {"file", b, "B", nil}, {"newest", "newest", "C", nil}, {"oldest", "oldest", "A", nil}, {"table", "newest", "B", map[string]string{"Note": b, "InputField": b}}} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "merged.jwlibrary")
			r, err := Merge(t.Context(), []string{a, b, c}, out, MergeOptions{Prefer: tc.prefer, TablePrefer: tc.prefs, Device: "Example merged device"})
			if err != nil {
				t.Fatal(err)
			}
			merged, err := Open(t.Context(), out)
			if err != nil {
				t.Fatal(err)
			}
			defer merged.Close()
			if err = merged.Validate(t.Context()); err != nil {
				t.Fatal(err)
			}
			var value string
			if err = merged.DB.QueryRowContext(t.Context(), "SELECT Content FROM Note WHERE Guid='shared-note'").Scan(&value); err != nil || value != tc.want {
				t.Fatalf("content=%q err=%v", value, err)
			}
			if r.After["Location"] != 1 || r.After["Note"] != 2 || r.After["TagMap"] != 2 || r.After["UserMark"] != 2 || (len(r.Overlaps) == 0 && tc.prefer != b) || len(r.Conflicts) == 0 {
				t.Fatalf("report: %+v", r)
			}
			var start int
			wantStart := 5
			if tc.prefer == b {
				wantStart = 2
			}
			if err = merged.DB.QueryRowContext(t.Context(), "SELECT StartToken FROM BlockRange JOIN UserMark USING(UserMarkId) WHERE UserMarkGuid='overlapping'").Scan(&start); err != nil || start != wantStart {
				t.Fatalf("uncovered start=%d err=%v", start, err)
			}
			if merged.Manifest.Backup.Device != "Example merged device" {
				t.Fatal("device name")
			}
		})
	}
	before, _ := os.ReadFile(a)
	r, err := Merge(t.Context(), []string{a, b}, "", MergeOptions{DryRun: true})
	if err != nil || !r.DryRun {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(a)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("input changed")
	}
}

func TestMediaAndPlaylist(t *testing.T) {
	seed := func(db *sql.DB) {
		execTest(t, db, `INSERT INTO Location(LocationId,DocumentId,Type) VALUES(71,123456,0); INSERT INTO PlaylistItemAccuracy VALUES(0,'Invented precise'); INSERT INTO IndependentMedia VALUES(80,'invented.bin','media.bin','application/octet-stream','synthetic-hash'); INSERT INTO PlaylistItem VALUES(90,'Invented item',0,10,0,0,'media.bin'); INSERT INTO PlaylistItemIndependentMediaMap VALUES(90,80,10); INSERT INTO PlaylistItemLocationMap VALUES(90,71,1,10); INSERT INTO PlaylistItemMarker VALUES(100,90,'Invented marker',0,10,0); INSERT INTO PlaylistItemMarkerBibleVerseMap VALUES(100,42); INSERT INTO PlaylistItemMarkerParagraphMap VALUES(100,123456,1,0); INSERT INTO Tag VALUES(111,2,'Invented playlist'); INSERT INTO TagMap(TagMapId,PlaylistItemId,TagId,Position) VALUES(112,90,111,0)`)
	}
	a := fixture(t, "media-a", "2026-01-01T00:00:00Z", seed, map[string][]byte{"media.bin": []byte("invented A")})
	b := fixture(t, "media-b", "2026-02-01T00:00:00Z", seed, map[string][]byte{"media.bin": []byte("invented B")})
	c := fixture(t, "media-c", "2026-03-01T00:00:00Z", seed, map[string][]byte{"media.bin": []byte("invented A")})
	out := filepath.Join(t.TempDir(), "merged.jwlibrary")
	r, err := Merge(t.Context(), []string{a, b, c}, out, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.After["IndependentMedia"] != 2 || r.After["PlaylistItem"] != 2 || r.After["PlaylistItemMarker"] != 2 || r.After["PlaylistItemMarkerBibleVerseMap"] != 2 || len(r.RenamedMedia[b]) != 1 {
		t.Fatalf("media merge: %+v", r)
	}
}

func TestMergeErrors(t *testing.T) {
	a := fixture(t, "alpha", "2026-01-01T00:00:00Z", nil, nil)
	b := fixture(t, "beta", "2026-02-01T00:00:00Z", nil, nil)
	for _, tc := range []struct {
		inputs []string
		opts   MergeOptions
		output string
	}{{[]string{a}, MergeOptions{}, ""}, {[]string{a, a}, MergeOptions{}, ""}, {[]string{a, b}, MergeOptions{Prefer: "missing", DryRun: true}, ""}, {[]string{a, b}, MergeOptions{TablePrefer: map[string]string{"Tag": "missing"}, DryRun: true}, ""}, {[]string{a, b}, MergeOptions{}, ""}, {[]string{a, b}, MergeOptions{}, a}} {
		if _, err := Merge(t.Context(), tc.inputs, tc.output, tc.opts); err == nil {
			t.Fatal("expected error")
		}
	}
	c := fixture(t, "different", "2026-03-01T00:00:00Z", func(db *sql.DB) { execTest(t, db, "CREATE INDEX invented_extra ON Tag(Name)") }, nil)
	if _, err := Merge(t.Context(), []string{a, c}, "", MergeOptions{DryRun: true}); err == nil {
		t.Fatal("different schema accepted")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Open(canceled, a); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestOpenRejectsInvalid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string][]byte
	}{{"path", map[string][]byte{"../bad": nil}}, {"manifest", map[string][]byte{"manifest.json": []byte("invalid")}}, {"version", map[string][]byte{"manifest.json": []byte(`{"version":1,"userDataBackup":{"schemaVersion":15}}`)}}} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "bad.zip")
			writeZip(t, p, tc.files)
			if _, err := Open(t.Context(), p); err == nil {
				t.Fatal("accepted invalid archive")
			}
		})
	}
	for _, tc := range []struct {
		name  string
		seed  func(*sql.DB)
		files map[string][]byte
	}{{"orphan", func(db *sql.DB) { execTest(t, db, `INSERT INTO UserMark VALUES(1,1,999,0,'orphan',1)`) }, nil}, {"missing media", func(db *sql.DB) {
		execTest(t, db, `INSERT INTO IndependentMedia VALUES(1,'invented','missing','application/octet-stream','hash')`)
	}, nil}, {"wrong version", func(db *sql.DB) { execTest(t, db, `PRAGMA user_version=15`) }, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixture(t, "bad", "2026-01-01T00:00:00Z", tc.seed, tc.files)
			if _, err := Open(t.Context(), p); err == nil {
				t.Fatal("accepted invalid database")
			}
		})
	}
	p := fixture(t, "hash", "2026-01-01T00:00:00Z", nil, nil)
	z, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range z.File {
		r, _ := f.Open()
		files[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	z.Close()
	files["userData.db"] = []byte("modified")
	bad := filepath.Join(t.TempDir(), "bad.zip")
	writeZip(t, bad, files)
	if _, err = Open(t.Context(), bad); err == nil {
		t.Fatal("bad hash accepted")
	}
}

func TestSubtract(t *testing.T) {
	for _, tc := range []struct {
		a, b    row
		count   int
		overlap bool
	}{{row{"StartToken": int64(0), "EndToken": int64(10)}, row{"StartToken": int64(3), "EndToken": int64(7)}, 2, true}, {row{"StartToken": int64(0), "EndToken": int64(2)}, row{"StartToken": int64(3), "EndToken": int64(7)}, 1, false}, {row{}, row{"StartToken": int64(3), "EndToken": int64(7)}, 0, true}, {row{"StartToken": int64(0), "EndToken": int64(2)}, row{}, 0, true}} {
		r, overlap := subtract(tc.a, tc.b)
		if len(r) != tc.count || overlap != tc.overlap {
			t.Fatalf("subtract=%v %v", r, overlap)
		}
	}
}

func TestTagMembershipPositions(t *testing.T) {
	a := fixture(t, "positions-a", "2026-01-01T00:00:00Z", func(db *sql.DB) {
		seedRows(t, db, 0, "A")
		execTest(t, db, `UPDATE TagMap SET Position=5; INSERT INTO Note(NoteId,Guid,Content) VALUES(20,'other-a','Invented second'); INSERT INTO TagMap(TagMapId,NoteId,TagId,Position) VALUES(21,20,5,2)`)
	}, nil)
	b := fixture(t, "positions-b", "2026-02-01T00:00:00Z", func(db *sql.DB) {
		seedRows(t, db, 100, "A")
		execTest(t, db, `INSERT INTO Note(NoteId,Guid,Content) VALUES(120,'other-b','Invented third'); INSERT INTO TagMap(TagMapId,NoteId,TagId,Position) VALUES(121,120,105,1)`)
	}, nil)
	out := filepath.Join(t.TempDir(), "merged.jwlibrary")
	if _, err := Merge(t.Context(), []string{a, b}, out, MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	merged, err := Open(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer merged.Close()
	for guid, want := range map[string]int{"shared-note": 5, "other-a": 2, "other-b": 6} {
		var got int
		if err = merged.DB.QueryRowContext(t.Context(), "SELECT Position FROM TagMap JOIN Note USING(NoteId) WHERE Guid=?", guid).Scan(&got); err != nil || got != want {
			t.Fatal(guid, got, err)
		}
	}
}

func TestResolverAndContainedMark(t *testing.T) {
	a := fixture(t, "alpha", "2026-01-01T00:00:00Z", func(db *sql.DB) { seedRows(t, db, 0, "A") }, nil)
	b := fixture(t, "beta", "2026-02-01T00:00:00Z", func(db *sql.DB) {
		seedRows(t, db, 100, "B")
		execTest(t, db, `INSERT INTO UserMark VALUES(120,2,101,0,'contained',1); INSERT INTO BlockRange VALUES(121,1,4,1,3,120); INSERT INTO Note(NoteId,Guid,UserMarkId,LocationId,Content) VALUES(122,'attached',120,101,'Invented linked text'); UPDATE BlockRange SET EndToken=3 WHERE UserMarkId=102`)
	}, nil)
	out := filepath.Join(t.TempDir(), "merged.jwlibrary")
	r, err := Merge(t.Context(), []string{a, b}, out, MergeOptions{Resolve: func(_ Conflict, _, second map[string]any) (map[string]any, error) { return second, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if r.After["UserMark"] != 1 || r.After["Note"] != 2 {
		t.Fatal(r)
	}
	merged, err := Open(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer merged.Close()
	var s string
	merged.DB.QueryRowContext(t.Context(), "SELECT Value FROM InputField").Scan(&s)
	if s != "B" {
		t.Fatal(s)
	}
}
