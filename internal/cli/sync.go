package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func (a *app) syncCmd() *cobra.Command {
	var issue, file string
	var force bool
	cmd := &cobra.Command{
		Use:   "sync <símbolo>...",
		Short: "Descarga (con caché y checksum), descifra e indexa publicaciones",
		Long: `Descarga el JWPUB de cada símbolo desde la API pub-media de jw.org, verifica su MD5,
lo descifra e indexa en la biblioteca. Si la copia local ya coincide con el checksum
de la CDN no descarga nada.

Símbolos útiles: mwb (Guía de actividades, con --issue AAAAMM), w (La Atalaya de
estudio, con --issue AAAAMM; las anteriores a 2016 usan AAAAMMDD), nwtsty (Biblia
de estudio), it (Perspicacia), wcg, lmd, th, jr, gl, lff, ijwia, sjj...`,
		Example: `  jwlib sync mwb --issue 202609
  jwlib sync w --issue 202607
  jwlib sync nwtsty it wcg
  jwlib sync w --issue 20130115
  jwlib sync --archivo ~/Descargas/mwb_S_202609.jwpub mwb --issue 202609`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			if file != "" {
				if len(args) != 1 {
					return fmt.Errorf("con --archivo indica un solo símbolo")
				}
				stats, err := st.IndexLocal(file, args[0], issue, a.lang)
				if err != nil {
					return err
				}
				if a.jsonOut {
					return a.printJSON(map[string]any{"clave": store.PubKey(args[0], a.lang, store.NormalizeIssue(issue)), "resumen": stats.String(), "indexado_ms": stats.Elapsed.Milliseconds()})
				}
				a.printf("✓ %s indexado desde %s en %s: %s\n", store.PubKey(args[0], a.lang, store.NormalizeIssue(issue)), file, ms(stats.Elapsed), stats)
				return nil
			}
			var results []*store.SyncResult
			var failed []string
			for _, sym := range args {
				res, err := a.syncOne(strings.TrimSpace(sym), issue, force)
				if err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", sym, err))
					if !a.jsonOut {
						fmt.Fprintf(a.err, "✗ %s: %v\n", sym, err)
					}
					continue
				}
				results = append(results, res)
				if a.jsonOut {
					continue
				}
				switch {
				case res.UpToDate:
					a.printf("✓ %s al día (%s, md5 %s)\n  %s\n", res.Key, mb(res.Size), res.MD5, res.Title)
				default:
					dl := "copia en caché verificada"
					if res.Downloaded {
						dl = fmt.Sprintf("descargado %s en %s", mb(res.Size), ms(res.Download))
					}
					a.printf("✓ %s · %s · indexado en %s\n  %s\n  %s\n", res.Key, dl, ms(res.Stats.Elapsed), res.Title, res.Summary)
				}
			}
			if a.jsonOut {
				if err := a.printJSON(map[string]any{"sincronizados": results, "errores": failed}); err != nil {
					return err
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d de %d publicaciones fallaron", len(failed), len(args))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&issue, "issue", "", "número de la publicación periódica: AAAAMM (o AAAAMMDD antes de 2016)")
	cmd.Flags().BoolVar(&force, "forzar", false, "descargar e indexar aunque la copia local esté al día")
	cmd.Flags().StringVar(&file, "archivo", "", "indexar un .jwpub local en vez de descargarlo")
	return cmd
}

func (a *app) pubsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "pubs",
		Aliases: []string{"biblioteca", "lista"},
		Short:   "Lista las publicaciones de la biblioteca local",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			pubs, err := st.Pubs()
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(pubs)
			}
			if len(pubs) == 0 {
				a.printf("La biblioteca %s está vacía. Empieza con: jwlib sync mwb --issue AAAAMM\n", a.libDir)
				return nil
			}
			var total int64
			for _, p := range pubs {
				total += p.Size
				a.printf("%-18s %-8s %5d docs  %9s  %s\n", p.Key, p.MepsSymbol, p.Docs, mb(p.Size), p.Title)
			}
			a.printf("%d publicaciones, %s en caché en %s\n", len(pubs), mb(total), st.PubsDir())
			return nil
		},
	}
}
