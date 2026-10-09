package backup

import (
	"database/sql"
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
	execTest(t, db, `CREATE TABLE pub(id INTEGER PRIMARY KEY,meps_symbol TEXT,symbol TEXT,meps_lang INTEGER,issue_tag INTEGER);
CREATE TABLE doc(docid INTEGER PRIMARY KEY,pub_id INTEGER,title TEXT,html TEXT);
INSERT INTO pub VALUES(1,'fictional','fictional',0,20260100);
INSERT INTO doc VALUES(123456,1,'Invented document','<p data-pid="4"><span class="parNum">99</span>Green robots, with purple hats, count exactly seven imaginary moons.</p><p data-pid="5">Blue robots carry tiny cubes.</p><textarea id="tt11"></textarea>');`)
	return db
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
