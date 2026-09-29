package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func (a *app) searchCmd() *cobra.Command {
	var pubs string
	var limit int
	var withBible bool
	cmd := &cobra.Command{
		Use:     `search "<query>"`,
		Aliases: []string{"buscar", "s"},
		Short:   "Full-text search (FTS5) across the library",
		Long: `Searches the decrypted text of every synced publication. All the words must land in
the same paragraph; accents are ignored; "in quotes" matches the exact phrase and
word* matches by prefix. Returns the best paragraph of each document: docid,
publication, title, paragraph and snippet.`,
		Example: `  pubkit search "cistern"
  pubkit search "cistern" --pub it,w
  pubkit search "\"accurate knowledge\"" --limit 10
  pubkit search "mire cistern" --bible`,
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
				return a.printJSON(map[string]any{"query": args[0], "fts": store.FTSQuery(args[0]), "documents": hits, "verses": verses})
			}
			if len(hits) == 0 && len(verses) == 0 {
				pl, _ := st.Pubs()
				if len(pl) == 0 {
					return errors.New("the library is empty: sync something first (pubkit sync it w nwtsty)")
				}
				a.printf("Nothing found for %q in %d publications.\n", args[0], len(pl))
				return nil
			}
			for _, h := range hits {
				par := fmt.Sprintf("pid %d", h.PID)
				if h.Num > 0 {
					par += fmt.Sprintf(", par. %d", h.Num)
				}
				a.printf("%-11d %-7s %s (%s", h.DocID, h.Pub, h.Title, par)
				if h.Matches > 1 {
					a.printf(", %d paragraphs match", h.Matches)
				}
				a.printf(")\n            %s\n", h.Snippet)
			}
			if len(verses) > 0 {
				a.printf("\nBible verses:\n")
				for _, v := range verses {
					bk, _ := bible.BookByNum(v.Book)
					a.printf("  %s %d:%d  %s\n", bk.Short, v.Chapter, v.Verse, v.Snippet)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pubs, "pub", "", "limit to publications (comma-separated symbols: it,w,w13,nwtsty,mwb)")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of documents")
	cmd.Flags().BoolVar(&withBible, "bible", false, "search the Bible text as well")
	return cmd
}
