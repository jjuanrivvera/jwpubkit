package jwpub_test

import (
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/jwpub"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

func TestOpenSyntheticJWPUB(t *testing.T) {
	path := testutil.Build(t, t.TempDir(), testutil.Pub{
		Symbol: "w26", Undated: "w", Year: 2026, IssueTag: 20260700, Title: "Prueba",
		Docs:      []testutil.Doc{{ID: 0, MepsID: 2026485, Class: 40, Title: "Doc", HTML: `<p id="p1" data-pid="1">¿Quién me tocó?</p>`}},
		ImageName: "2026485_univ_cnt_1.jpg", ImageData: []byte("not really a jpeg"),
	})
	jf, err := jwpub.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer jf.Close()
	if got := jf.Card.String(); got != "1_w26_2026_20260700" {
		t.Errorf("card %q", got)
	}
	var blob []byte
	if err := jf.DB.QueryRow(`SELECT Content FROM Document WHERE MepsDocumentId=2026485`).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	html, err := jf.Decrypt(blob)
	if err != nil || html != `<p id="p1" data-pid="1">¿Quién me tocó?</p>` {
		t.Fatalf("decrypt: %q %v", html, err)
	}
	if !jf.HasFile("2026485_univ_cnt_1.jpg") {
		t.Error("image missing from contents")
	}
	if b, err := jf.ReadFile("2026485_univ_cnt_1.jpg"); err != nil || string(b) != "not really a jpeg" {
		t.Errorf("ReadFile: %q %v", b, err)
	}
	if _, err := jf.ReadFile("nope.jpg"); err == nil {
		t.Error("expected an error for a missing file")
	}
	if !jf.HasTable("Document") || jf.HasTable("NoSuchTable") || jf.Col("Document", "Nope") != "NULL" {
		t.Error("schema helpers")
	}
}

func TestOpenRejectsNonZip(t *testing.T) {
	if _, err := jwpub.Open("jwpub_test.go"); err == nil {
		t.Fatal("expected an error")
	}
}
