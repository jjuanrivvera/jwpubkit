package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := "# a machine's own choices\n" +
		"language = S\n" +
		"media_dir: ~/media\n" +
		"\n" +
		"library=\"/srv/library\"\n" +
		"unknown = ignored\n" +
		"not a setting\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWPUBKIT_CONFIG", path)

	c := Load()
	if c.Language != "S" {
		t.Errorf("Language = %q", c.Language)
	}
	if c.Library != "/srv/library" {
		t.Errorf("Library = %q", c.Library)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, "media"); c.MediaDir != want {
		t.Errorf("MediaDir = %q, want %q", c.MediaDir, want)
	}
	if c.Path != path {
		t.Errorf("Path = %q", c.Path)
	}
}

// No settings file is the ordinary case, not a failure.
func TestLoadWithoutAFile(t *testing.T) {
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(t.TempDir(), "absent"))
	c := Load()
	if c != (Config{}) {
		t.Errorf("a missing file should leave everything unset, got %+v", c)
	}
}

func TestBackupStorePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config")
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("JWPUBKIT_CONFIG", file)
	if err := os.WriteFile(file, []byte("backup_store: '~/private-backups'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.BackupStore != filepath.Join(dir, "private-backups") || got.Path != file {
		t.Fatalf("%+v", got)
	}
}
