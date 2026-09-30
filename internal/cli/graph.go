package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

// graphCmd walks the connections the publications already state.
//
// There is a graph in a JWPUB library and it was written by hand, over decades:
// which article quotes which verse, what a Bible's margin points at, what one
// work extracts from another, which video an article embeds, which term a study
// note defines. None of it is inferred and none of it needs a model. What it
// needed was a way to walk it, and a way to see which signal each step came from.
func (a *app) graphCmd() *cobra.Command {
	var perRelation, coCitation int
	var only string
	cmd := &cobra.Command{
		Use:     "graph <reference|docid>",
		Aliases: []string{"links", "grafo"},
		Short:   "Walk the connections the library already records, from a passage or a document",
		Long: `Takes a Bible reference or a document id and reports what the library connects it to,
with the signal each connection came from.

From a passage: the documents that quote it, what its margin points at, the
passages quoted alongside it in the same paragraph, the videos embedded in the
documents that quote it, and the terms its study notes define.

From a document: what quotes it, what it refers to, the videos it embeds and the
passages it quotes.

Nothing here is guessed. Every edge names its source table, so an answer can be
checked. A much-quoted verse is capped rather than truncated silently — raise the
caps if you want the long list.`,
		Example: `  pubkit graph "Jer 38:6"
  pubkit graph "John 3:16" --json
  pubkit graph 1102025901
  pubkit graph "Ps 23:1" --only cited-alongside --per-relation 40`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			lim := store.GraphLimits{PerRelation: perRelation, CoCitation: coCitation}

			var edges []store.Edge
			var subject string
			if docid, convErr := strconv.Atoi(args[0]); convErr == nil {
				d, err := st.Doc(docid)
				if err != nil {
					return err
				}
				subject = fmt.Sprintf("docid %d · %s", docid, d.Title)
				if edges, err = st.DocGraph(docid, lim); err != nil {
					return err
				}
			} else {
				ranges, err := bible.Parse(args[0])
				if err != nil {
					return err
				}
				subject = bible.FormatList(ranges)
				for _, r := range ranges {
					part, err := st.VerseGraph(r.FirstID(), r.LastID(), lim)
					if err != nil {
						return err
					}
					edges = append(edges, part...)
				}
			}
			if only != "" {
				kept := edges[:0]
				for _, e := range edges {
					if e.Relation == only {
						kept = append(kept, e)
					}
				}
				edges = kept
			}

			if a.jsonOut {
				if edges == nil {
					edges = []store.Edge{}
				}
				return a.printJSON(map[string]any{
					"subject": subject, "edges": edges,
					"limits": map[string]int{"per_relation": lim.PerRelation, "co_citation": lim.CoCitation},
				})
			}
			if len(edges) == 0 {
				a.printf("%s: the library records no connections for this.\n", subject)
				return nil
			}
			a.printf("%s · %d connections\n", subject, len(edges))
			last := ""
			for _, e := range edges {
				if e.Relation != last {
					a.printf("\n%s  (%s)\n", e.Relation, e.Source)
					last = e.Relation
				}
				a.printf("  %s\n", graphLine(e))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&perRelation, "per-relation", 25, "maximum edges of each kind")
	cmd.Flags().IntVar(&coCitation, "co-citation", 15, "maximum passages quoted alongside this one")
	cmd.Flags().StringVar(&only, "only", "", "keep just one relation (cited-by, points-to, cited-alongside, embeds-video, defines-term…)")
	return cmd
}

// graphLine renders one edge as a single readable line, whatever is on its far end.
func graphLine(e store.Edge) string {
	switch {
	case e.Reference != "":
		s := e.Reference
		if e.Weight > 0 {
			s += fmt.Sprintf("  ×%d", e.Weight)
		}
		if e.Text != "" {
			s += "  «" + e.Text + "»"
		}
		return s
	case e.VideoKey != "":
		s := e.VideoKey
		if e.Text != "" {
			s += "  " + e.Text
		}
		if e.Weight > 0 {
			s += fmt.Sprintf("  (%d cues indexed)", e.Weight)
		}
		return s
	case e.Term != "":
		s := e.Term
		if e.Text != "" {
			s += ": " + truncate(e.Text, 96)
		}
		return s
	default:
		s := e.Cite
		if s == "" {
			s = fmt.Sprintf("docid %d", e.DocID)
		}
		if e.Title != "" {
			s += "  " + e.Title
		}
		if e.Kind != "" && e.Kind != "article" {
			s += " [" + e.Kind + "]"
		}
		return s
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
