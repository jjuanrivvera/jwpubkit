// Package config reads the small settings file that keeps a machine's choices
// out of its login environment.
//
// The environment is not a reliable place for them: a variable exported from a
// shell profile never reaches cron, a systemd service or a program launched by
// another program, and the failure is silent — the tool simply behaves as if
// nothing was set. That is exactly how a Spanish library ended up downloading
// an English workbook.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Config is what a machine can decide once instead of on every command line.
type Config struct {
	Language    string
	Library     string
	MediaDir    string
	BackupStore string
	// Path is the file these came from, empty when there is none.
	Path string
}

// Path is the settings file: $JWPUBKIT_CONFIG, else $XDG_CONFIG_HOME/pubkit/config,
// else ~/.config/pubkit/config.
func Path() string {
	if p := os.Getenv("JWPUBKIT_CONFIG"); p != "" {
		return p
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "pubkit", "config")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pubkit", "config")
}

// Load reads the settings file. A missing file is not an error: it is the
// ordinary case, and every setting simply stays unset.
func Load() Config {
	path := Path()
	f, err := os.Open(path) // #nosec G304 -- the path the user configured, by design
	if err != nil {
		return Config{}
	}
	defer f.Close()

	c := Config{Path: path}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
			if !ok {
				continue
			}
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "language", "lang":
			c.Language = value
		case "library", "home":
			c.Library = expand(value)
		case "media_dir", "media-dir":
			c.MediaDir = expand(value)
		case "backup_store", "backup-store":
			c.BackupStore = expand(value)
		}
	}
	return c
}

// expand resolves a leading ~ so a settings file can be written the way people
// write paths.
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
