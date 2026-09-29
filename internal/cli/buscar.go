package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func (a *app) buscarCmd() *cobra.Command {
	var pubs string
	var limit int
	var withBible bool
	cmd := &cobra.Command{
		Use:     `buscar "<consulta>"`,
		Aliases: []string{"search", "s"},
		Short:   "Búsqueda de texto completo (FTS5) en la biblioteca",
		Long: `Busca en el texto descifrado de todas las publicaciones sincronizadas. Todas las
palabras deben aparecer en el mismo párrafo; las tildes no importan; "entre comillas"
busca la frase exacta y palabra* busca por prefijo. Devuelve el mejor párrafo de cada
documento: docid, publicación, título, párrafo y extracto.`,
		Example: `  jwlib buscar "Ébed-Mélec"
  jwlib buscar "cisterna" --pub it,w
  jwlib buscar "\"conocimiento exacto\"" --limite 10
  jwlib buscar "fango cisterna" --biblia`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			var filter []string
			for _, p := range strings.Split(pubs, ",") {
				if p = strings.TrimSpace(p); p != "" {
					filter = append(filter, p)
				}
			}
			hits, err := st.Search(args[0], filter, limit)
			if err != nil {
				return err
			}
			var verses []store.VerseHit
			if withBible {
				if verses, err = st.SearchVerses(args[0], limit); err != nil {
					return err
				}
			}
			if a.jsonOut {
				if hits == nil {
					hits = []store.SearchHit{}
				}
				if verses == nil {
					verses = []store.VerseHit{}
				}
				return a.printJSON(map[string]any{"consulta": args[0], "fts": store.FTSQuery(args[0]), "documentos": hits, "versiculos": verses})
			}
			if len(hits) == 0 && len(verses) == 0 {
				pl, _ := st.Pubs()
				if len(pl) == 0 {
					return errors.New("la biblioteca está vacía: sincroniza algo primero (jwlib sync it w nwtsty)")
				}
				a.printf("Sin resultados para %q en %d publicaciones.\n", args[0], len(pl))
				return nil
			}
			for _, h := range hits {
				par := fmt.Sprintf("pid %d", h.PID)
				if h.Num > 0 {
					par += fmt.Sprintf(", párr. %d", h.Num)
				}
				a.printf("%-11d %-7s %s (%s", h.DocID, h.Pub, h.Title, par)
				if h.Matches > 1 {
					a.printf(", %d párrafos coinciden", h.Matches)
				}
				a.printf(")\n            %s\n", h.Snippet)
			}
			if len(verses) > 0 {
				a.printf("\nVersículos (TNM):\n")
				for _, v := range verses {
					bk, _ := bible.BookByNum(v.Book)
					a.printf("  %s %d:%d  %s\n", bk.Short, v.Chapter, v.Verse, v.Snippet)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pubs, "pub", "", "limitar a publicaciones (símbolos separados por coma: it,w,w13,nwtsty,mwb)")
	cmd.Flags().IntVar(&limit, "limite", 20, "máximo de documentos")
	cmd.Flags().BoolVar(&withBible, "biblia", false, "buscar también en el texto de la Biblia")
	return cmd
}
