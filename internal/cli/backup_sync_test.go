package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/backup"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

func TestBackupSyncStatusAndAliases(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(dir, "missing"))
	t.Setenv("JWPUBKIT_HOME", dir)
	store := filepath.Join(dir, "backups")
	for _, args := range [][]string{{"backup", "status", "--store", store}, {"respaldo", "estado", "--almacen", store, "--json"}, {"backup", "sync", "--help"}, {"respaldo", "sincronizar", "--help"}} {
		out := runCLI(t, args...)
		if !strings.Contains(out, "master") && !strings.Contains(out, "history") {
			t.Fatal(out)
		}
	}
	for _, args := range [][]string{{"backup", "sync"}, {"backup", "sync", "missing", "--history-limit", "-1"}, {"backup", "sync", "missing", "--store", store}, {"backup", "sync", "--watch", filepath.Join(dir, "absent"), "--store", store}} {
		a := &app{out: &bytes.Buffer{}, err: &bytes.Buffer{}, origins: map[string]string{}, ctx: t.Context()}
		cmd := a.rootCmd()
		cmd.SetArgs(args)
		if err := cmd.ExecuteContext(t.Context()); err == nil {
			t.Fatal(args)
		}
	}
}

func syntheticBackup(t *testing.T, texts ...string) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "userData.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(t.Context(), "PRAGMA journal_mode=MEMORY; PRAGMA synchronous=OFF;"+testutil.BackupSchema); err != nil {
		t.Fatal(err)
	}
	for i, text := range texts {
		if _, err = db.ExecContext(t.Context(), "INSERT INTO Note(NoteId,Guid,Content,Created,LastModified) VALUES(?,?,?,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')", i+1, fmt.Sprintf("invented-%d", i), text); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	var m backup.Manifest
	m.Version = 1
	m.Backup.Schema = 16
	m.Backup.Database = "userData.db"
	m.Backup.Hash = hex.EncodeToString(hash[:])
	m.Backup.Device = "Invented device"
	m.Backup.LastModified = "2026-01-01T00:00:00Z"
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "synthetic.jwlibrary")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, b := range map[string][]byte{"manifest.json": manifest, "userData.db": data} {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(b); e != nil {
			t.Fatal(e)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBackupSyncCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, []byte("backup_store = "+store+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWPUBKIT_CONFIG", cfg)
	t.Setenv("JWPUBKIT_HOME", dir)
	t.Setenv("JWPUBKIT_BACKUP_STORE", "")
	t.Setenv("JWLIB_BACKUP_STORE", "")
	initial := syntheticBackup(t, "Invented original")
	var first backup.SyncResult
	if err := json.Unmarshal([]byte(runCLI(t, "respaldo", "sincronizar", initial, "--historial", "2", "--nombre-dispositivo", "Invented master")), &first); err != nil {
		t.Fatal(err)
	}
	if first.Status.Master != filepath.Join(store, "master.jwlibrary") || first.Status.Device != "Invented master" {
		t.Fatal(first.Status)
	}
	incoming := syntheticBackup(t, "Invented revision", "Invented addition")
	var next backup.SyncResult
	if err := json.Unmarshal([]byte(runCLI(t, "backup", "sync", incoming, "--preferir", "incoming", "--prefer-notes", "incoming", "--historial", "2")), &next); err != nil {
		t.Fatal(err)
	}
	if next.Status.Counts["Note"] != 2 || len(next.Status.History) != 1 {
		t.Fatal(next.Status)
	}
	if output := runCLI(t, "respaldo", "estado"); !strings.Contains(output, "Last synchronization:") || !strings.Contains(output, "Note") {
		t.Fatal(output)
	}
	var status backup.SyncStatus
	if err := json.Unmarshal([]byte(runCLI(t, "backup", "status", "--json")), &status); err != nil {
		t.Fatal(err)
	}
	if status.Counts["Note"] != 2 {
		t.Fatal(status)
	}
}

func TestBackupWatchCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "incoming")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(dir, "absent"))
	t.Setenv("JWPUBKIT_HOME", dir)
	incoming := syntheticBackup(t, "Invented watched note")
	data, err := os.ReadFile(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(folder, "new.jwlibrary"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	a := &app{out: writer, err: &bytes.Buffer{}, ctx: ctx, origins: map[string]string{}}
	root := a.rootCmd()
	root.SetArgs([]string{"respaldo", "sincronizar", "--vigilar", folder, "--almacen", filepath.Join(dir, "store")})
	done := make(chan error, 1)
	go func() { e := root.ExecuteContext(ctx); writer.CloseWithError(e); done <- e }()
	var event backup.WatchEvent
	if err = json.NewDecoder(reader).Decode(&event); err != nil {
		t.Fatal(err)
	}
	cancel()
	if event.Error != "" || event.Result == nil || event.Result.Imported != 1 {
		t.Fatal(event)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		select {
		case err = <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("watch did not stop after cancellation")
		}
	}
	if _, err = os.Stat(filepath.Join(folder, "procesados", "new.jwlibrary")); err != nil {
		t.Fatal(err)
	}
}
