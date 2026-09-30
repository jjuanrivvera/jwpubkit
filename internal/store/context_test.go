package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// WithContext must scope the reads of one command without taking the shared
// handle with it: cancelling a command has to stop that command's queries, and
// nothing else.
func TestWithContextScopesTheReadsNotTheHandle(t *testing.T) {
	s := queryFixture(t)

	ctx, cancel := context.WithCancel(t.Context())
	scoped := s.WithContext(ctx)
	if scoped == s {
		t.Fatal("WithContext should hand back a copy, not the same store")
	}
	if scoped.DB != s.DB {
		t.Error("the copy should share the database handle; reopening it would lock")
	}

	if _, err := scoped.MediaOf(101); err != nil {
		t.Fatalf("a scoped read should work: %v", err)
	}

	cancel()
	if _, err := scoped.MediaOf(101); !errors.Is(err, context.Canceled) {
		t.Errorf("after cancelling, the scoped read should stop: %v", err)
	}
	// The original store is untouched: its reads still answer.
	if _, err := s.MediaOf(101); err != nil {
		t.Errorf("cancelling one command must not disable the library: %v", err)
	}
}

// A dry run must leave an absent library absent. Opening read-only on a path
// that is not there is an error, not an empty database created on the way.
func TestOpenReadOnlyDoesNotCreateALibrary(t *testing.T) {
	dir := t.TempDir()

	if _, err := OpenReadOnly(t.Context(), dir, "E"); err == nil {
		t.Fatal("opening a library that is not there should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("nothing should have been created, found %s", filepath.Join(dir, e.Name()))
	}
}

// And on a library that does exist it reads, but refuses to write.
func TestOpenReadOnlyReadsButDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "E")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO pub(id, key, symbol, issue, lang, title, year) VALUES(1,'w_E_202601','w','202601','E','Invented Magazine',2026)`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := OpenReadOnly(t.Context(), dir, "E")
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()

	pub, err := ro.PubByKey("w_E_202601")
	if err != nil || pub == nil {
		t.Fatalf("a read-only store should still read: %+v, %v", pub, err)
	}
	if ro.Lang != "E" || ro.Dir != dir || ro.Path == "" {
		t.Errorf("the read-only store should know where it is: %+v", ro)
	}
	if _, err := ro.DB.Exec(`INSERT INTO pub(id, key, symbol, issue, lang, year) VALUES(2,'x','x','','E',2026)`); err == nil {
		t.Error("a read-only store must refuse a write")
	}
}
