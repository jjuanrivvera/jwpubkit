package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// indexOne puts one invented publication into a library and returns its directory.
func indexOne(t *testing.T, lang string) string {
	t.Helper()
	dir := t.TempDir()
	pub := testutil.Pub{
		Symbol: "th", Undated: "th", Year: 2026, Title: "A publication",
		Docs: []testutil.Doc{{ID: 0, MepsID: 1102018437, Class: 40, Title: "A document",
			HTML: `<p id="p1" data-pid="1">Invented text.</p>`}},
	}
	file := testutil.Build(t, t.TempDir(), pub)
	st, err := store.Open(dir, lang)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.IndexLocal(file, "th", "", lang); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// This is the mistake the command exists to prevent, written down as a test: a
// library that has just been indexed keeps its content in the -wal, so the
// database file itself can be a few kilobytes. Size is not emptiness.
func TestIndexedLibraryIsNotEmptyEvenWhenTheDBFileIsTiny(t *testing.T) {
	dir := indexOne(t, "E")
	st, err := store.Open(dir, "E")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pubs, err := st.Pubs()
	if err != nil || len(pubs) != 1 {
		t.Fatalf("the library should hold one publication: %+v %v", pubs, err)
	}
	if total := st.Size(); total < 1000 {
		t.Errorf("Size() = %d, want the -wal counted too", total)
	}
	if got := st.Files(); len(got) != 3 {
		t.Errorf("Files() = %v, want the database with its -wal and -shm", got)
	}
}

// Without --yes the command must destroy nothing, however loudly it describes
// what it would do.
func TestDropReportsWithoutRemoving(t *testing.T) {
	dir := indexOne(t, "E")
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("JWPUBKIT_LANG", "")

	out := runCLI(t, "--library", dir, "--language", "E", "drop")
	if !strings.Contains(out, "Nothing was removed") {
		t.Errorf("a dry run must say so:\n%s", out)
	}
	if !strings.Contains(out, "th_E") {
		t.Errorf("it should name what it would remove:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "jwlib.E.db")); err != nil {
		t.Errorf("the database was removed without --yes: %v", err)
	}
}

// With --yes every file of that database goes, and no orphan is left behind to be
// replayed onto whatever is created next.
func TestDropRemovesAllThreeFiles(t *testing.T) {
	dir := indexOne(t, "E")
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("JWPUBKIT_LANG", "")

	out := runCLI(t, "--library", dir, "--language", "E", "drop", "--yes")
	if !strings.Contains(out, "removed") {
		t.Errorf("it should report what it removed:\n%s", out)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := filepath.Join(dir, "jwlib.E.db"+suffix)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived the drop", filepath.Base(path))
		}
	}
	// The cache is shared and stays unless it was asked for.
	entries, _ := os.ReadDir(filepath.Join(dir, "pubs"))
	if len(entries) == 0 {
		t.Error("the downloaded publications should be kept by default")
	}
	// And another language is untouched by a drop of this one.
	if _, err := os.Stat(filepath.Join(dir, "jwlib.db")); err == nil {
		t.Error("dropping E should not have created or kept the default-language file")
	}
}

func TestDropWithCacheRemovesTheDownloads(t *testing.T) {
	dir := indexOne(t, "E")
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("JWPUBKIT_LANG", "")

	runCLI(t, "--library", dir, "--language", "E", "drop", "--cache", "--yes")
	entries, _ := os.ReadDir(filepath.Join(dir, "pubs"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jwpub") {
			t.Errorf("--cache should have removed %s", e.Name())
		}
	}
}
