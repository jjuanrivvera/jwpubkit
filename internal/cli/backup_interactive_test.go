package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/backup"
)

func TestBackupWordDiff(t *testing.T) {
	if got := wordDiff("purple robots count", "green robots dance"); !strings.Contains(got, "[-purple-]") || !strings.Contains(got, "[+green+]") || !strings.Contains(got, "robots") {
		t.Fatal(got)
	}
	if got := wordDiff("same", "same"); got != "same" {
		t.Fatal(got)
	}
	if !strings.Contains(wordDiff(strings.Repeat("a ", 1001), "b"), "[+") {
		t.Fatal("large diff")
	}
}

func TestResolveBackupConflict(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"a\n", "A"}, {"b\n", "B"}, {"m\n", "A\n\n--- merged alternative ---\n\nB"}, {"s\n", ""}, {"\n", ""}, {"", ""}, {"invalid\nb\n", "B"}} {
		t.Run(tc.input, func(t *testing.T) {
			var out bytes.Buffer
			first := map[string]any{"Content": "A", "Title": "Invented A"}
			second := map[string]any{"Content": "B", "Title": "Invented B"}
			result, err := resolveBackupConflict(t.Context(), bufio.NewReader(strings.NewReader(tc.input)), &out, backup.Conflict{Table: "Note"}, first, second)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if result != nil {
					t.Fatal(result)
				}
			} else if result["Content"] != tc.want {
				t.Fatal(result)
			}
			if !strings.Contains(out.String(), "Word diff") {
				t.Fatal("missing diff")
			}
		})
	}
	var out bytes.Buffer
	r, err := resolveBackupConflict(t.Context(), bufio.NewReader(strings.NewReader("b\n")), &out, backup.Conflict{Table: "InputField"}, map[string]any{"Value": "A"}, map[string]any{"Value": "B"})
	if err != nil || r["Value"] != "B" {
		t.Fatal(r, err)
	}
}

func TestEditBackupText(t *testing.T) {
	t.Setenv("EDITOR", "")
	if _, err := editBackupText(t.Context(), "invented"); err == nil {
		t.Fatal("missing editor accepted")
	}
	t.Setenv("EDITOR", "false")
	if _, err := editBackupText(t.Context(), "invented"); err == nil {
		t.Fatal("failed editor accepted")
	}
	t.Setenv("EDITOR", "true")
	if text, err := editBackupText(t.Context(), "invented"); err != nil || text != "invented" {
		t.Fatal(text, err)
	}
}

func TestEditorArgs(t *testing.T) {
	args, err := editorArgs(`"/path with spaces/editor" --wait 'two words' escaped\ value`)
	if err != nil || len(args) != 4 || args[0] != "/path with spaces/editor" || args[2] != "two words" || args[3] != "escaped value" {
		t.Fatal(args, err)
	}
	for _, command := range []string{"", `"unfinished`, `editor\`, `''`} {
		if _, err = editorArgs(command); err == nil {
			t.Fatal("invalid editor command")
		}
	}
}

func TestBackupCommands(t *testing.T) {
	a := &app{out: &bytes.Buffer{}, err: &bytes.Buffer{}, origins: map[string]string{}, ctx: t.Context()}
	root := a.rootCmd()
	for _, args := range [][]string{{"backup", "merge", "a", "b"}, {"backup", "merge", "a", "b", "--dry-run", "--interactive"}, {"backup", "annotate", "a"}, {"respaldo", "inspeccionar", "missing"}, {"backup", "annotate", "a", "-o", "b", "--plan", "missing"}} {
		root.SetArgs(args)
		if err := root.ExecuteContext(t.Context()); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}
