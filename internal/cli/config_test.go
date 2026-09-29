package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/config"
)

// run executes the CLI with args and returns what it printed.
func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	a := &app{}
	var out bytes.Buffer
	a.out, a.err = &out, &out
	a.ctx = t.Context()
	a.cfg, a.origins = config.Load(), map[string]string{}
	root := a.rootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	return out.String()
}

// The bug this guards: the language was set in a shell profile, which a cron
// job, a service or a program launched by another program never reads, so a
// Spanish library quietly downloaded an English workbook. A settings file is
// read wherever the process runs.
func TestLanguageComesFromTheSettingsFileWithoutAnyEnvironment(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, []byte("language = S\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWPUBKIT_CONFIG", cfg)
	t.Setenv("JWPUBKIT_LANG", "")
	t.Setenv("JWLIB_HOME", "")
	t.Setenv("JWPUBKIT_HOME", dir)

	out := runCLI(t, "config")
	if !strings.Contains(out, "language   S") {
		t.Errorf("the settings file should decide the language:\n%s", out)
	}
	if !strings.Contains(out, cfg) {
		t.Errorf("the output should name the file it read:\n%s", out)
	}
}

// Order: a flag beats the environment, the environment beats the file, the file
// beats the default. Each step is what somebody will rely on.
func TestSettingPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, []byte("language = S\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWPUBKIT_CONFIG", cfg)
	t.Setenv("JWPUBKIT_HOME", dir)

	t.Setenv("JWPUBKIT_LANG", "")
	if out := runCLI(t, "config"); !strings.Contains(out, "language   S") {
		t.Errorf("file should win over the default:\n%s", out)
	}
	t.Setenv("JWPUBKIT_LANG", "F")
	if out := runCLI(t, "config"); !strings.Contains(out, "language   F") ||
		!strings.Contains(out, "JWPUBKIT_LANG") {
		t.Errorf("environment should win over the file:\n%s", out)
	}
	if out := runCLI(t, "--language", "J", "config"); !strings.Contains(out, "language   J") ||
		!strings.Contains(out, "--language") {
		t.Errorf("the flag should win over everything:\n%s", out)
	}
}

// With nothing set anywhere the tool says so, rather than looking configured.
func TestNoSettingsFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JWPUBKIT_CONFIG", filepath.Join(dir, "absent"))
	t.Setenv("JWPUBKIT_LANG", "")
	t.Setenv("JWPUBKIT_HOME", dir)

	out := runCLI(t, "config")
	if !strings.Contains(out, "(not present)") {
		t.Errorf("a missing settings file should be stated:\n%s", out)
	}
	if !strings.Contains(out, "language   E") {
		t.Errorf("the built-in default should apply:\n%s", out)
	}
}
