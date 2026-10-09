package backup

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncMasterBatchHistoryAndIdempotence(t *testing.T) {
	store := filepath.Join(t.TempDir(), "store")
	empty, err := Status(t.Context(), store)
	if err != nil || empty.Initialized {
		t.Fatalf("%+v %v", empty, err)
	}
	if _, err = os.Stat(store); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("status created an absent store")
	}
	original := ancestorFixture(t)
	opts := SyncOptions{MergeOptions: MergeOptions{Prefer: "incoming", Device: "Invented master"}, HistoryLimit: 1}
	initial, err := Sync(t.Context(), store, []string{original}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Imported != 1 || !initial.Status.Initialized || initial.Status.Device != "Invented master" || initial.Status.Counts["Note"] != 1 {
		t.Fatalf("%+v", initial)
	}
	base := initial.Status.Base
	left := descendant(t, base, "left", func(db *sql.DB) {
		execTest(t, db, `DELETE FROM TagMap WHERE NoteId=4; DELETE FROM Note; INSERT INTO Note(NoteId,Guid,Content) VALUES(20,'invented-left','Invented left addition')`)
	})
	right := descendant(t, base, "right", func(db *sql.DB) {
		execTest(t, db, `UPDATE InputField SET Value='Invented right edit'; INSERT INTO Note(NoteId,Guid,Content) VALUES(21,'invented-right','Invented right addition')`)
	})
	batch, err := Sync(t.Context(), store, []string{left, right}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Imported != 2 || batch.Status.Counts["Note"] != 2 || len(batch.Status.History) != 1 {
		t.Fatalf("%+v", batch)
	}
	a, err := Open(t.Context(), batch.Status.Master)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	err = a.DB.QueryRowContext(t.Context(), "SELECT Value FROM InputField").Scan(&value)
	a.Close()
	if err != nil || value != "Invented right edit" {
		t.Fatalf("%s %v", value, err)
	}
	master, _ := os.ReadFile(batch.Status.Master)
	pointer, _ := os.ReadFile(filepath.Join(store, "current.json"))
	baseBytes, _ := os.ReadFile(batch.Status.Base)
	if !bytes.Equal(master, baseBytes) {
		t.Fatal("next master and base differ")
	}
	for _, report := range batch.Reports {
		for path := range report.Before {
			if strings.Contains(path, ".sync-") {
				t.Fatal("disposable path in report", path)
			}
		}
	}
	same, err := Sync(t.Context(), store, []string{left, right}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if same.Imported != 0 || same.AlreadySynced != 2 || same.Status.LastSync != batch.Status.LastSync {
		t.Fatalf("%+v", same)
	}
	failed := filepath.Join(t.TempDir(), "invalid.jwlibrary")
	if err = os.WriteFile(failed, []byte("Invented invalid archive"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := descendant(t, batch.Status.Master, "fresh", func(db *sql.DB) { execTest(t, db, `UPDATE InputField SET Value='Invented unsaved'`) })
	if _, err = Sync(t.Context(), store, []string{fresh, failed}, opts); err == nil {
		t.Fatal("invalid batch committed")
	}
	after, _ := os.ReadFile(batch.Status.Master)
	afterPointer, _ := os.ReadFile(filepath.Join(store, "current.json"))
	if !bytes.Equal(pointer, afterPointer) {
		t.Fatal("failed batch changed the committed generation")
	}
	if !bytes.Equal(master, after) {
		t.Fatal("failed batch changed the master")
	}
	for i := range 3 {
		incoming := descendant(t, batch.Status.Master, fmt.Sprintf("revision-%d", i), func(db *sql.DB) {
			execTest(t, db, "UPDATE InputField SET Value=?", fmt.Sprintf("Invented revision %d", i))
		})
		batch, err = Sync(t.Context(), store, []string{incoming}, opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch.Status.History) != 1 {
			t.Fatal(batch.Status.History)
		}
	}
	generations, err := os.ReadDir(filepath.Join(store, "history"))
	if err != nil || len(generations) != 2 {
		t.Fatalf("history rotation: %d %v", len(generations), err)
	}
	opts.HistoryLimit = 0
	if _, err = Sync(t.Context(), store, []string{fresh}, opts); err != nil {
		t.Fatal(err)
	}
	status, err := Status(t.Context(), store)
	if err != nil || len(status.History) != 0 || status.MasterDate == "" || status.LastSync == "" {
		t.Fatalf("%+v %v", status, err)
	}
}

func TestSyncConflictDatesAndInputPreferences(t *testing.T) {
	var inputs []string
	for _, month := range []int{1, 3, 2} {
		stamp := fmt.Sprintf("2026-%02d-01T00:00:00Z", month)
		inputs = append(inputs, fixture(t, fmt.Sprintf("input-%d", month), stamp, func(db *sql.DB) { seedRows(t, db, 0, fmt.Sprintf("Invented month %d", month)) }, nil))
	}
	for _, tc := range []struct{ prefer, notes, wantNote, wantField string }{
		{"newest", "", "Invented month 3", "Invented month 3"},
		{"oldest", "", "Invented month 1", "Invented month 1"},
		{"newest", "oldest", "Invented month 1", "Invented month 3"},
		{inputs[1], "", "Invented month 3", "Invented month 3"},
	} {
		label := tc.prefer + tc.notes
		if filepath.IsAbs(tc.prefer) {
			label = "input-path"
		}
		t.Run(label, func(t *testing.T) {
			result, err := Sync(t.Context(), t.TempDir(), inputs, SyncOptions{MergeOptions: MergeOptions{Prefer: tc.prefer, TablePrefer: map[string]string{"Note": tc.notes}}, HistoryLimit: 2})
			if err != nil {
				t.Fatal(err)
			}
			a, err := Open(t.Context(), result.Status.Master)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if a.Manifest.Name != "master.jwlibrary" {
				t.Fatal(a.Manifest.Name)
			}
			var note, field string
			if err = a.DB.QueryRowContext(t.Context(), "SELECT Content FROM Note").Scan(&note); err != nil {
				t.Fatal(err)
			}
			if err = a.DB.QueryRowContext(t.Context(), "SELECT Value FROM InputField").Scan(&field); err != nil {
				t.Fatal(err)
			}
			if note != tc.wantNote || field != tc.wantField {
				t.Fatalf("%s, %s", note, field)
			}
		})
	}
}

func TestSyncOlderAncestorAndInteractiveDeletion(t *testing.T) {
	original := ancestorFixture(t)
	for _, interactive := range []bool{false, true} {
		store := t.TempDir()
		opts := SyncOptions{MergeOptions: MergeOptions{Prefer: "master"}, HistoryLimit: 2}
		initial, err := Sync(t.Context(), store, []string{original}, opts)
		if err != nil {
			t.Fatal(err)
		}
		ancestor := descendant(t, initial.Status.Master, "distributed", nil)
		removed := descendant(t, ancestor, "removed", func(db *sql.DB) { execTest(t, db, `DELETE FROM TagMap WHERE NoteId=4; DELETE FROM Note`) })
		revised := descendant(t, ancestor, "revised", func(db *sql.DB) { execTest(t, db, `UPDATE Note SET Content='Invented edited'`) })
		calls := 0
		if interactive {
			opts.Resolve = func(c Conflict, a, b map[string]any) (map[string]any, error) {
				calls++
				if strings.Contains(c.Winner, ".sync-") || strings.Contains(c.Other, ".sync-") {
					t.Fatal(c)
				}
				return b, nil
			}
		} else {
			opts.TablePrefer = map[string]string{"Note": "incoming"}
		}
		result, err := Sync(t.Context(), store, []string{removed, revised}, opts)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status.Counts["Note"] != 1 || interactive && calls == 0 {
			t.Fatalf("%+v", result)
		}
		a := descendant(t, ancestor, "addition-a", func(db *sql.DB) {
			execTest(t, db, `INSERT INTO Note(NoteId,Guid,Content) VALUES(20,'invented-a','Invented A')`)
		})
		b := descendant(t, ancestor, "addition-b", func(db *sql.DB) {
			execTest(t, db, `INSERT INTO Note(NoteId,Guid,Content) VALUES(21,'invented-b','Invented B')`)
		})
		opts.Base = ancestor
		opts.Resolve = nil
		if _, err = Sync(t.Context(), store, []string{a}, opts); err != nil {
			t.Fatal(err)
		}
		final, err := Sync(t.Context(), store, []string{b}, opts)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status.Counts["Note"] != 3 {
			t.Fatal(final.Status.Counts)
		}
	}
}

func TestSyncRecoveryLocksAndErrors(t *testing.T) {
	original := ancestorFixture(t)
	store := t.TempDir()
	opts := SyncOptions{HistoryLimit: 2}
	result, err := Sync(t.Context(), store, []string{original}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(result.Status.Master); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(result.Status.Base); err != nil {
		t.Fatal(err)
	}
	recovered, err := Status(t.Context(), store)
	if err != nil || !recovered.Initialized {
		t.Fatalf("%+v %v", recovered, err)
	}
	if err = os.WriteFile(recovered.Master, []byte("Invented overwritten export"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Status(t.Context(), store); err != nil {
		t.Fatal("retained generation was modified by its export", err)
	}
	a, err := Open(t.Context(), recovered.Master)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	_, lock, err := acquireStore(store, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Status(t.Context(), store); err == nil {
		t.Fatal("concurrent status accepted")
	}
	if _, err = Sync(t.Context(), store, []string{original}, opts); err == nil {
		t.Fatal("concurrent sync accepted")
	}
	lock.Close()
	for _, tc := range []struct {
		inputs []string
		opts   SyncOptions
	}{
		{nil, opts}, {[]string{original}, SyncOptions{HistoryLimit: -1}},
		{[]string{original}, SyncOptions{MergeOptions: MergeOptions{DryRun: true}}},
		{[]string{original}, SyncOptions{MergeOptions: MergeOptions{Prefer: "missing"}}},
		{[]string{recovered.Master}, opts},
	} {
		if _, err = Sync(t.Context(), store, tc.inputs, tc.opts); err == nil {
			t.Fatalf("invalid sync accepted: %+v", tc)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = Sync(ctx, store, []string{original}, opts); err == nil {
		t.Fatal("canceled sync accepted")
	}
	if err = os.WriteFile(filepath.Join(store, "current.json"), []byte(`{"generation":"../bad"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Status(t.Context(), store); err == nil {
		t.Fatal("unsafe generation accepted")
	}
	if _, err = Status(t.Context(), ""); err == nil {
		t.Fatal("empty store accepted")
	}
}

func TestWatchStableBatchAndInvalidNeighbor(t *testing.T) {
	root := t.TempDir()
	store, folder := filepath.Join(root, "store"), filepath.Join(root, "incoming")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	original := ancestorFixture(t)
	opts := SyncOptions{HistoryLimit: 2}
	initial, err := Sync(t.Context(), store, []string{original}, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		incoming := descendant(t, initial.Status.Master, fmt.Sprintf("side-%d", i), func(db *sql.DB) {
			execTest(t, db, `INSERT INTO Note(NoteId,Guid,Content) VALUES(?,?,?)`, 20+i, fmt.Sprintf("invented-%d", i), "Invented addition")
		})
		if _, err = copyBackup(incoming, filepath.Join(folder, fmt.Sprintf("side-%d.jwlibrary", i))); err != nil {
			t.Fatal(err)
		}
	}
	bad := filepath.Join(folder, "broken.jwlibrary")
	if err = os.WriteFile(bad, []byte("Invented incomplete backup"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	invalid, imported := 0, 0
	err = Watch(ctx, store, folder, opts, time.Millisecond, func(e WatchEvent) error {
		if e.Error != "" {
			invalid++
			return nil
		}
		if e.Result != nil {
			imported = e.Result.Imported
			if e.Result.Status.Counts["Note"] != 3 {
				t.Fatal(e.Result.Status.Counts)
			}
			cancel()
		}
		return nil
	})
	if err != nil || invalid != 1 || imported != 2 {
		t.Fatalf("invalid=%d imported=%d %v", invalid, imported, err)
	}
	files, err := os.ReadDir(filepath.Join(folder, "procesados"))
	if err != nil || len(files) != 2 {
		t.Fatalf("%d %v", len(files), err)
	}
	if _, err = os.Stat(bad); err != nil {
		t.Fatal("invalid backup was moved")
	}
}

func TestWatchRetriesBusyStoreAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	store, folder := filepath.Join(root, "store"), filepath.Join(root, "incoming")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	original := ancestorFixture(t)
	input := filepath.Join(folder, "snapshot.jwlibrary")
	if _, err := copyBackup(original, input); err != nil {
		t.Fatal(err)
	}
	_, lock, err := acquireStore(store, true)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	busy, success := false, 0
	err = Watch(ctx, store, folder, SyncOptions{HistoryLimit: 1}, time.Millisecond, func(e WatchEvent) error {
		if e.Error != "" {
			if !strings.Contains(e.Error, "busy") {
				t.Fatal(e.Error)
			}
			busy = true
			lock.Close()
			return nil
		}
		success++
		if success == 1 {
			if _, err := copyBackup(original, input); err != nil {
				t.Fatal(err)
			}
		} else {
			if e.Result.AlreadySynced != 1 || e.Result.Imported != 0 {
				t.Fatal(e.Result)
			}
			cancel()
		}
		return nil
	})
	if err != nil || !busy || success != 2 {
		t.Fatalf("busy=%v success=%d %v", busy, success, err)
	}
	files, err := os.ReadDir(filepath.Join(folder, "procesados"))
	if err != nil || len(files) != 2 {
		t.Fatalf("%d %v", len(files), err)
	}
}

func TestWatchCompletedCopyAndValidation(t *testing.T) {
	root := t.TempDir()
	store, folder := filepath.Join(root, "store"), filepath.Join(root, "incoming")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	original := ancestorFixture(t)
	input := filepath.Join(folder, "partial.jwlibrary")
	if err := os.WriteFile(input, []byte("Partial invented zip"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	completed := false
	err := Watch(ctx, store, folder, SyncOptions{}, time.Millisecond, func(e WatchEvent) error {
		if e.Error != "" {
			b, err := os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(input, b, 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			completed = true
			cancel()
		}
		return nil
	})
	if err != nil || !completed {
		t.Fatalf("completed=%v %v", completed, err)
	}
	for _, pair := range [][2]string{{store, store}, {root, folder}, {folder, root}, {store, filepath.Join(root, "absent")}, {store, original}} {
		if _, err = ValidateWatch(pair[0], pair[1]); err == nil {
			t.Fatal(pair)
		}
	}
	if err = Watch(t.Context(), store, folder, SyncOptions{}, 0, func(WatchEvent) error { return nil }); err == nil {
		t.Fatal("zero interval accepted")
	}
	if err = moveProcessed(original, filepath.Join(folder, "procesados"), "wrong hash"); err == nil {
		t.Fatal("changed snapshot removed")
	}
}

func TestSyncRetainsGuidlessLineage(t *testing.T) {
	original := ancestorFixture(t)
	store := t.TempDir()
	opts := SyncOptions{MergeOptions: MergeOptions{Prefer: "master"}, HistoryLimit: 2}
	initial, err := Sync(t.Context(), store, []string{original}, opts)
	if err != nil {
		t.Fatal(err)
	}
	ancestor := descendant(t, initial.Status.Master, "distributed", nil)
	renamed := descendant(t, ancestor, "renamed", func(db *sql.DB) { execTest(t, db, "UPDATE Tag SET Name='Invented renamed' WHERE TagId=5") })
	changed, err := Sync(t.Context(), store, []string{renamed}, opts)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Open(t.Context(), changed.Status.Master)
	if err != nil {
		t.Fatal(err)
	}
	var id int
	err = a.DB.QueryRowContext(t.Context(), "SELECT TagId FROM Tag WHERE Name='Invented renamed'").Scan(&id)
	a.Close()
	if err != nil || id != 5 {
		t.Fatalf("ancestor ID lost: %d %v", id, err)
	}
	removed := descendant(t, ancestor, "removed", func(db *sql.DB) { execTest(t, db, "DELETE FROM TagMap WHERE TagId=5; DELETE FROM Tag WHERE TagId=5") })
	opts.Base = ancestor
	last, err := Sync(t.Context(), store, []string{removed}, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range last.Reports {
		for _, c := range r.Conflicts {
			if c.Table == "Tag" && c.Kind == "delete_edit" {
				found = true
			}
		}
	}
	if !found || last.Status.Counts["Tag"] != 2 {
		t.Fatalf("rename lost its ancestry: %+v", last)
	}
}
