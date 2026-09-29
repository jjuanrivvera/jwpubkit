// Package cli wires the jwlib commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

// Version is set at build time with -ldflags "-X jwlib/internal/cli.Version=…".
var Version = "dev"

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

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "jwlib",
		Short: "Lee las publicaciones de la JW (JWPUB) sin pasar por wol",
		Long: `jwlib descarga publicaciones en formato JWPUB desde la CDN abierta de jw.org,
las descifra y las indexa en una biblioteca local (SQLite + FTS5) para consultar
la reunión de la semana, versículos de la TNM con sus notas, búsquedas,
documentos completos, imágenes y subtítulos de videos.

Biblioteca: ~/.local/share/jwlib (cámbiala con --biblioteca o JWLIB_HOME).`,
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
	pf := root.PersistentFlags()
	pf.BoolVar(&a.jsonOut, "json", false, "salida en JSON")
	pf.StringVar(&a.libDir, "biblioteca", store.DefaultDir(), "carpeta de la biblioteca local")
	pf.BoolVar(&a.offline, "sin-red", false, "no usar la red (no sincroniza ni consulta la CDN)")
	pf.StringVar(&a.lang, "idioma", "S", "idioma de las publicaciones (código de jw.org; S = español)")
	pf.BoolVarP(&a.quiet, "silencioso", "q", false, "no mostrar progreso en stderr")
	root.AddCommand(a.syncCmd(), a.semanaCmd(), a.versiculoCmd(), a.buscarCmd(), a.docCmd(), a.imagenCmd(), a.subtitulosCmd(), a.pubsCmd(), a.expedienteCmd())
	return root
}

func (a *app) store() (*store.Store, error) {
	if a.st != nil {
		return a.st, nil
	}
	st, err := store.Open(a.libDir)
	if err != nil {
		return nil, fmt.Errorf("abriendo la biblioteca %s: %w", a.libDir, err)
	}
	a.st = st
	return st, nil
}

func (a *app) client() *cdn.Client {
	if a.cdn == nil {
		a.cdn = cdn.New(a.lang)
	}
	return a.cdn
}

// logf prints progress on stderr unless --silencioso.
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

var errOffline = errors.New("hace falta la red y se pidió --sin-red")

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
	return strings.Replace(fmt.Sprintf("%.1f s", d.Seconds()), ".", ",", 1)
}

func mb(n int64) string {
	return strings.Replace(fmt.Sprintf("%.1f MB", float64(n)/1e6), ".", ",", 1)
}
