package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/backup"
)

func (a *app) backupResolver(cmd *cobra.Command) (backup.Resolver, error) {
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return nil, errors.New("--interactive requires a TTY; use --prefer <input|newest|oldest> for unattended merges")
	}
	reader := bufio.NewReader(os.Stdin)
	return func(c backup.Conflict, first, second map[string]any) (map[string]any, error) {
		return resolveBackupConflict(cmd.Context(), reader, a.err, c, first, second)
	}, nil
}

func resolveBackupConflict(ctx context.Context, reader *bufio.Reader, out io.Writer, c backup.Conflict, first, second map[string]any) (map[string]any, error) {
	field := "Content"
	if c.Table == "InputField" {
		field = "Value"
	}
	str := func(v any) string {
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	a, b := str(first[field]), str(second[field])
	fmt.Fprintf(out, "\n%s conflict %s\nA (preferred): %s\nB: %s\n", c.Table, c.Key, c.Winner, c.Other)
	if c.Table == "Note" {
		fmt.Fprintf(out, "Title A: %s\nTitle B: %s\n", str(first["Title"]), str(second["Title"]))
	}
	fmt.Fprintf(out, "A: %s\nB: %s\nWord diff (-A, +B):\n%s\n", a, b, wordDiff(a, b))
	for {
		fmt.Fprint(out, "[a] keep A, [b] keep B, [m] concatenate, [e] edit in $EDITOR, [s/Enter] skip decision (keep preference): ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil
			}
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "s", "skip":
			return nil, nil
		case "a":
			return first, nil
		case "b":
			return second, nil
		case "m":
			first[field] = a + "\n\n--- merged alternative ---\n\n" + b
			return first, nil
		case "e":
			text, err := editBackupText(ctx, a+"\n\n--- merged alternative ---\n\n"+b)
			if err != nil {
				return nil, err
			}
			first[field] = text
			return first, nil
		default:
			fmt.Fprintln(out, "Choose a, b, m, e or s.")
		}
	}
}

func editBackupText(ctx context.Context, text string) (string, error) {
	editor := os.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		return "", errors.New("set $EDITOR before choosing edit")
	}
	f, err := os.CreateTemp("", "pubkit-conflict-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	args, err := editorArgs(editor)
	if err != nil {
		return "", err
	}
	// Parse editor arguments without a shell so note text and environment expansions cannot execute.
	cmd := exec.CommandContext(ctx, args[0], append(args[1:], f.Name())...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("editor: %w", err)
	}
	b, err := os.ReadFile(f.Name())
	return string(b), err
}

func editorArgs(command string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, c := range command {
		if escaped {
			word.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			started = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			started = true
			continue
		}
		if unicode.IsSpace(c) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(c)
		started = true
	}
	if quote != 0 || escaped {
		return nil, errors.New("$EDITOR has an unmatched quote or escape")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("$EDITOR has no executable")
	}
	return args, nil
}

func wordDiff(a, b string) string {
	x, y := strings.Fields(a), strings.Fields(b)
	// Bound memory for long imported notes; originals are still shown in full.
	if len(x) > 1000 || len(y) > 1000 {
		return "[-" + a + "-]\n[+" + b + "+]"
	}
	dp := make([][]int, len(x)+1)
	for i := range dp {
		dp[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var words []string
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			words = append(words, x[i])
			i++
			j++
		case i < len(x) && (j == len(y) || dp[i+1][j] >= dp[i][j+1]):
			words = append(words, "[-"+x[i]+"-]")
			i++
		default:
			words = append(words, "[+"+y[j]+"+]")
			j++
		}
	}
	return strings.Join(words, " ")
}
