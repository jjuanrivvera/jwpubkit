// Package cli wires the pubkit commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

// Version is set at build time with -ldflags "-X github.com/jjuanrivvera/jwpubkit/internal/cli.Version=…".
var Version = "dev"

// defaultLang is the jw.org language symbol used when nothing else says otherwise.
// E is English; every other language is reachable with --language.
const defaultLang = "E"

type app struct {
	jsonOut bool
	libDir  string
	offline bool
	lang    string
	quiet   bool

	st  *store.Store
	cdn *cdn.Client
	out io.Writer
	err io.Writer
	ctx context.Context
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a := &app{out: os.Stdout, err: os.Stderr, ctx: ctx}
	root := a.rootCmd()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// legacyFlags maps the Spanish flag names this CLI shipped with to their current
// names, so a script written against an older version keeps working.
var legacyFlags = map[string]string{
	"biblioteca":  "library",
	"idioma":      "language",
	"silencioso":  "quiet",
	"sin-red":     "offline",
	"archivo":     "file",
	"biblia":      "bible",
	"citas":       "citations",
	"extractos":   "extracts",
	"formato":     "format",
	"forzar":      "force",
	"limite":      "limit",
	"listar":      "list",
	"salida":      "output",
	"sin-atalaya": "no-watchtower",
	"sin-citas":   "no-citations",
	"sin-notas":   "no-notes",
	"tiempos":     "timings",
}

func normalizeFlag(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	if current, ok := legacyFlags[name]; ok {
		return pflag.NormalizedName(current)
	}
	return pflag.NormalizedName(name)
}

func (a *app) rootCmd() *cobra.Command {
	name := filepath.Base(os.Args[0])
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "pubkit"
	}
	root := &cobra.Command{
		Use:   name,
		Short: "Read JWPUB publications from a local library",
		Long: `` + name + ` downloads publications in JWPUB format from the open jw.org CDN,
decrypts them and indexes them into a local library (SQLite + FTS5) you can query
offline: the week's meeting, Bible passages with their notes, full-text search,
whole documents, images and video subtitles.

It redistributes nothing: it works on what you downloaded, on your machine.

Library: ~/.local/share/jwlib (change it with --library, JWPUBKIT_HOME or JWLIB_HOME).`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if a.st != nil {
				return a.st.Close()
			}
			return nil
		},
	}
	root.SetGlobalNormalizationFunc(normalizeFlag)
	pf := root.PersistentFlags()
	pf.BoolVar(&a.jsonOut, "json", false, "output JSON")
	pf.StringVar(&a.libDir, "library", store.DefaultDir(), "local library directory")
	pf.BoolVar(&a.offline, "offline", false, "stay off the network (no sync, no CDN lookups)")
	pf.StringVar(&a.lang, "language", langDefault(), "publication language (jw.org symbol: E english, S spanish, F french…)")
	pf.BoolVarP(&a.quiet, "quiet", "q", false, "do not print progress on stderr")
	root.AddCommand(a.syncCmd(), a.weekCmd(), a.verseCmd(), a.searchCmd(), a.docCmd(), a.imageCmd(), a.subtitlesCmd(), a.pubsCmd(), a.dossierCmd(), a.completionCmd(), a.versionCmd())
	return root
}

// langDefault lets a library that holds one language say so once, in the
// environment, instead of every command line repeating --language.
func langDefault() string {
	if l := os.Getenv("JWPUBKIT_LANG"); l != "" {
		return l
	}
	return defaultLang
}

func (a *app) store() (*store.Store, error) {
	if a.st != nil {
		return a.st, nil
	}
	st, err := store.Open(a.libDir, a.lang)
	if err != nil {
		return nil, fmt.Errorf("opening the library %s: %w", a.libDir, err)
	}
	// Opening the library teaches the bible package the names of every language
	// it holds a Bible in; this picks which of them references are printed in.
	bible.UseLanguage(a.lang)
	a.st = st
	return st, nil
}

func (a *app) client() *cdn.Client {
	if a.cdn == nil {
		a.cdn = cdn.New(a.lang)
	}
	return a.cdn
}

// logf prints progress on stderr unless --quiet.
func (a *app) logf(format string, args ...any) {
	if a.quiet {
		return
	}
	fmt.Fprintf(a.err, "· "+format+"\n", args...)
}

func (a *app) printJSON(v any) error {
	enc := json.NewEncoder(a.out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *app) printf(format string, args ...any) { fmt.Fprintf(a.out, format, args...) }

var errOffline = errors.New("this needs the network and --offline was given")

// syncOne syncs a publication, printing progress.
func (a *app) syncOne(symbol, issue string, force bool) (*store.SyncResult, error) {
	if a.offline {
		return nil, errOffline
	}
	st, err := a.store()
	if err != nil {
		return nil, err
	}
	return st.Sync(a.ctx, a.client(), symbol, issue, force, a.logf)
}

func ms(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}

func mb(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/1e6)
}
