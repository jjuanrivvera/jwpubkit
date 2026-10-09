package backup

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func libraryFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	execTest(t, db, `CREATE TABLE pub(id INTEGER PRIMARY KEY,meps_symbol TEXT,symbol TEXT,undated_symbol TEXT,meps_lang INTEGER,issue_tag INTEGER);
CREATE TABLE doc(docid INTEGER PRIMARY KEY,pub_id INTEGER,title TEXT,html TEXT);
INSERT INTO pub VALUES(1,'fictional26','fictional','fictional',12,20260100);
INSERT INTO doc VALUES(123456,1,'Invented document','<p data-pid="4"><span class="parNum">99</span>Green robots, with purple hats, count exactly seven imaginary moons.</p><p data-pid="5">Blue robots carry tiny cubes.</p><textarea id="tt11"></textarea>');`)
	return db
}

func TestAnnotationLocationUsesUndatedSymbolAndMEPSLanguage(t *testing.T) {
	lib := libraryFixture(t)
	execTest(t, lib, "UPDATE pub SET symbol='catalog-fictional'")
	// Reference rows model existing backup locations; publication text stays invented.
	input := fixture(t, "location-reference", "2026-01-01T00:00:00Z", func(db *sql.DB) {
		execTest(t, db, `INSERT INTO Location(LocationId,DocumentId,KeySymbol,MepsLanguage,IssueTagNumber,Type) VALUES
(10,123456,'fictional',12,20260100,0),
(11,123457,'fictional',12,20260100,0),
(12,123456,'fictional',0,20260100,0)`)
	}, nil)
	plan := AnnotationPlan{Highlights: []Highlight{{123456, 4, "purple hats", 1}}, Answers: []Answer{{123456, "tt11", "Invented response."}}}
	out := filepath.Join(t.TempDir(), "annotated.jwlibrary")
	if _, err := Annotate(t.Context(), input, out, lib, plan, ""); err != nil {
		t.Fatal(err)
	}
	a, err := Open(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var count int
	if err = a.DB.QueryRowContext(t.Context(), "SELECT count(*) FROM Location").Scan(&count); err != nil || count != 3 {
		t.Fatalf("existing location was not reused: count=%d err=%v", count, err)
	}
	for _, table := range []string{"UserMark", "InputField"} {
		var loc, language int64
		var symbol string
		err = a.DB.QueryRowContext(t.Context(), "SELECT LocationId,KeySymbol,MepsLanguage FROM "+table+" JOIN Location USING(LocationId)").Scan(&loc, &symbol, &language)
		if err != nil || loc != 10 || symbol != "fictional" || language != 12 {
			t.Fatalf("%s location=%d symbol=%q language=%d err=%v", table, loc, symbol, language, err)
		}
	}
	// A new document of the same publication must match the existing reference metadata too.
	v, _, err := document(t.Context(), lib, 123456)
	if err != nil {
		t.Fatal(err)
	}
	v["DocumentId"] = int64(123458)
	tx, err := a.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id, err := annotationLocation(t.Context(), tx, v)
	if err != nil {
		t.Fatal(err)
	}
	var same int
	err = tx.QueryRowContext(t.Context(), `SELECT count(*) FROM Location created JOIN Location reference
ON created.KeySymbol=reference.KeySymbol AND created.MepsLanguage=reference.MepsLanguage AND created.Type=reference.Type
WHERE created.LocationId=? AND reference.LocationId=11`, id).Scan(&same)
	if err != nil || same != 1 {
		t.Fatal("new location differs from the reference", err)
	}
}

func TestAnnotationSymbolFallback(t *testing.T) {
	lib := libraryFixture(t)
	for _, undated := range []any{"", nil} {
		execTest(t, lib, "UPDATE pub SET undated_symbol=?", undated)
		v, _, err := document(t.Context(), lib, 123456)
		if err != nil || v["KeySymbol"] != "fictional" || v["MepsLanguage"] != int64(12) {
			t.Fatal(v, err)
		}
	}
	execTest(t, lib, "UPDATE pub SET symbol=''")
	if _, _, err := document(t.Context(), lib, 123456); err == nil {
		t.Fatal("dated symbol used when no undated symbol is available")
	}
}

// Optional local integration checks use external backups; no private fixture is retained.
func TestAnnotationLocationsAgainstExternalBackup(t *testing.T) {
	backupPath, libraryPath, planPath := os.Getenv("PUBKIT_TEST_BACKUP"), os.Getenv("PUBKIT_TEST_LIBRARY"), os.Getenv("PUBKIT_TEST_ANNOTATION_PLAN")
	if backupPath == "" || libraryPath == "" || planPath == "" {
		t.Skip("set PUBKIT_TEST_BACKUP, PUBKIT_TEST_LIBRARY and PUBKIT_TEST_ANNOTATION_PLAN for private local verification")
	}
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var plan AnnotationPlan
	if err = json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	a, err := Open(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	lib, err := sql.Open("sqlite", "file:"+filepath.ToSlash(libraryPath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	ids := map[int64]bool{}
	for _, h := range plan.Highlights {
		ids[h.DocID] = true
	}
	for _, answer := range plan.Answers {
		ids[answer.DocID] = true
	}
	if len(ids) == 0 {
		t.Fatal("plan has no document locations")
	}
	out := filepath.Join(t.TempDir(), "external-reference-annotations.jwlibrary")
	if _, err = Annotate(t.Context(), backupPath, out, lib, plan, "Local annotation verification"); err != nil {
		t.Fatal(err)
	}
	generated, err := Open(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer generated.Close()
	for docid := range ids {
		v, _, err := document(t.Context(), lib, docid)
		if err != nil {
			t.Fatal(err)
		}
		var expectedLanguage int64
		if err = lib.QueryRowContext(t.Context(), "SELECT pub.meps_lang FROM pub JOIN doc ON doc.pub_id=pub.id WHERE doc.docid=?", docid).Scan(&expectedLanguage); err != nil {
			t.Fatal(err)
		}
		if v["MepsLanguage"] != expectedLanguage {
			t.Fatal("MEPS language differs from the source publication")
		}
		var actualSymbol string
		var actualLanguage, actualType int64
		if err = generated.DB.QueryRowContext(t.Context(), `SELECT KeySymbol,MepsLanguage,Type FROM Location WHERE DocumentId=? AND IssueTagNumber=? AND MepsLanguage=?`, docid, v["IssueTagNumber"], expectedLanguage).Scan(&actualSymbol, &actualLanguage, &actualType); err != nil {
			t.Fatal(err)
		}
		if actualSymbol != v["KeySymbol"] || actualLanguage != expectedLanguage || actualType != v["Type"] {
			t.Fatal("generated location differs from the source metadata")
		}
		var matches int
		if err = a.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM Location WHERE Type=? AND KeySymbol=? AND MepsLanguage=?`, actualType, actualSymbol, actualLanguage).Scan(&matches); err != nil {
			t.Fatal(err)
		}
		if matches == 0 {
			t.Fatal("annotation symbol and language do not match any existing backup location of the same type")
		}
	}
}

func TestAnnotate(t *testing.T) {
	lib := libraryFixture(t)
	input := fixture(t, "annotations", "2026-01-01T00:00:00Z", nil, nil)
	plan := AnnotationPlan{Highlights: []Highlight{{123456, 4, "purple hats", 1}, {123456, 5, "tiny cubes", 2}}, Note: &AnnotationNote{0, "Imaginary observation", "The invented robots prefer purple."}, Answers: []Answer{{123456, "tt11", "Seven imaginary moons."}}}
	out := filepath.Join(t.TempDir(), "annotated.jwlibrary")
	r, err := Annotate(t.Context(), input, out, lib, plan, "Invented prototype")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Experimental || len(r.Ranges) != 2 || r.Notes != 1 || r.Answers != 1 || r.Ranges[0].Start != 4 || r.Ranges[0].End != 5 {
		t.Fatal(r)
	}
	a, err := Open(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	i, err := a.Inspect(t.Context())
	if err != nil || i.Counts["Location"] != 1 || i.Counts["UserMark"] != 2 || i.Counts["Note"] != 1 || i.Counts["InputField"] != 1 {
		t.Fatal(i, err)
	}
	if _, err = Annotate(t.Context(), out, filepath.Join(t.TempDir(), "again.jwlibrary"), lib, plan, ""); err == nil {
		t.Fatal("duplicate highlights accepted")
	}
	answerOnly := AnnotationPlan{Answers: []Answer{{123456, "tt11", "Another invented answer."}}}
	if _, err = Annotate(t.Context(), out, filepath.Join(t.TempDir(), "again.jwlibrary"), lib, answerOnly, ""); err == nil {
		t.Fatal("duplicate answer accepted")
	}
	if _, err = Annotate(t.Context(), input, filepath.Join(t.TempDir(), "answer.jwlibrary"), lib, answerOnly, ""); err != nil {
		t.Fatal(err)
	}
}

func TestAnnotateRejectsInvalidPlans(t *testing.T) {
	lib := libraryFixture(t)
	input := fixture(t, "annotations", "2026-01-01T00:00:00Z", nil, nil)
	for _, plan := range []AnnotationPlan{{}, {Highlights: []Highlight{{123456, 4, "robots", 0}}}, {Highlights: []Highlight{{999, 4, "robots", 1}}}, {Highlights: []Highlight{{123456, 999, "robots", 1}}}, {Highlights: []Highlight{{123456, 4, "absent", 1}}}, {Note: &AnnotationNote{0, "Invented", "text"}}, {Answers: []Answer{{999, "tt11", "invented"}}}, {Answers: []Answer{{123456, "tt12", "invented"}}}, {Answers: []Answer{{123456, "tt11", " "}}}} {
		if _, err := Annotate(t.Context(), input, filepath.Join(t.TempDir(), "bad.jwlibrary"), lib, plan, ""); err == nil {
			t.Fatalf("invalid plan accepted: %+v", plan)
		}
	}
}

func TestTokensAndMarkup(t *testing.T) {
	text, err := ParagraphText(`<p data-pid="9"><span class="parNum">5</span>Imaginary <b>robots</b>, count&nbsp;7.<span class="fn">skip</span><ruby>X<rt>skip</rt></ruby><textarea>skip</textarea></p>`, 9)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Imaginary robots, count 7.X" {
		t.Fatal(text)
	}
	if !reflect.DeepEqual(Tokens("robots, hats!"), []string{"robots", ",", "hats", "!"}) {
		t.Fatal("punctuation tokenization")
	}
	if _, _, err = quoteRange("blue blue", "blue"); err == nil {
		t.Fatal("ambiguous quote accepted")
	}
	if _, _, err = quoteRange("blue", ""); err == nil {
		t.Fatal("empty quote accepted")
	}
	if _, err = ParagraphText("<p>invented</p>", 9); err == nil {
		t.Fatal("missing paragraph accepted")
	}
	if validTextTag(`<div id="tt1"></div>`, "tt1") || validTextTag(`<textarea id="tt1"></textarea>`, "invalid") {
		t.Fatal("invalid textarea accepted")
	}
}
