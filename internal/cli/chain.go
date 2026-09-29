package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
)

// chainCmd follows the marginal references out of a passage, as far as asked.
//
// One hop is what an open Bible shows. The reason to go further is that the
// marginal references are a graph the publications built by hand, and what sits
// two or three hops out is often the connection someone was looking for and
// could not have found by reading one page.
func (a *app) chainCmd() *cobra.Command {
	var hops, limit int
	cmd := &cobra.Command{
		Use:     `chain "<reference>"`,
		Aliases: []string{"xrefs", "cadena"},
		Short:   "Follow the marginal references out of a passage, with each verse's text",
		Long: `Walks the marginal references of a passage and prints every verse it reaches,
with the word that pointed there and how many references away it is.

The walk is breadth-first and visits a verse once, so a verse is reported at the
fewest references it is reachable by. That matters because the references point
both ways: without it the walk would not finish.`,
		Example: `  pubkit chain "Jer 38:6"
  pubkit chain "John 3:16" --hops 2
  pubkit chain "Ps 23:1" --hops 3 --limit 40 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			if !st.HasBible() {
				return fmt.Errorf("no Bible in the library: pubkit sync nwtsty")
			}
			ranges, err := bible.Parse(args[0])
			if err != nil {
				return err
			}
			type reported struct {
				Reference string `json:"reference"`
				From      string `json:"from,omitempty"`
				Hop       int    `json:"hop"`
				Anchor    string `json:"anchor,omitempty"`
				Text      string `json:"text"`
				URL       string `json:"url"`
			}
			var out []reported
			for _, r := range ranges {
				steps, err := st.Chain(r.FirstID(), r.LastID(), hops, limit)
				if err != nil {
					return err
				}
				for _, s := range steps {
					rep := reported{Hop: s.Hop, Anchor: s.Anchor, Text: s.Text}
					if rg, ok := bible.FromIDs(s.VerseID, s.VerseID); ok {
						rep.Reference = rg.String()
					}
					if rg, ok := bible.FromIDs(s.From, s.From); ok {
						rep.From = rg.String()
					}
					out = append(out, rep)
				}
			}
			if a.jsonOut {
				if out == nil {
					out = []reported{}
				}
				return a.printJSON(map[string]any{
					"reference": bible.FormatList(ranges), "hops": hops, "verses": out,
				})
			}
			if len(out) == 0 {
				a.printf("%s has no marginal references in this library.\n", bible.FormatList(ranges))
				return nil
			}
			a.printf("%s · %d verses within %d reference%s\n\n", bible.FormatList(ranges), len(out), hops, plural2(hops))
			for _, r := range out {
				a.printf("%d  %-14s %s\n", r.Hop, r.Reference, r.Text)
				if r.From != "" {
					a.printf("   %-14s ← %s «%s»\n", "", r.From, r.Anchor)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&hops, "hops", 1, "how many references deep to follow")
	cmd.Flags().IntVar(&limit, "limit", 200, "maximum number of verses to report")
	return cmd
}

func plural2(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
