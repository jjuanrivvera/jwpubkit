package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type verseResult struct {
	Ref         string           `json:"reference"`
	Ranges      []bible.Range    `json:"ranges"`
	Verses      []store.Verse    `json:"verses"`
	CitedBy     []store.Citation `json:"cited_in"`
	CitedTotal  int              `json:"citing_documents_total"`
	Translation string           `json:"translation"`
}

func (a *app) verseCmd() *cobra.Command {
	var citations int
	var noNotes, noCites bool
	cmd := &cobra.Command{
		Use:     `verse "<reference>"`,
		Aliases: []string{"versiculo", "versículo", "v"},
		Short:   "The Bible text verbatim, with its notes, cross references and who cites it",
		Long: `Prints the verses from the study Bible in your library exactly as they are, with
their footnotes, marginal references and study notes, plus the documents in the
library that cite the passage (the BibleCitation table of every synced publication).

References are parsed in the library's language: "Jer 38:6", "Jeremiah 38:1-13",
"1 Cor. 13:4-7", "Ps 23", "Jer 38:28-39:2", "Jer 38:6; 39:1, 4-6".`,
		Example: `  pubkit verse "Jer 38:6"
  pubkit verse "Jer 38:1-13" --no-notes
  pubkit verse "John 3:16" --citations 40 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// The library is opened first on purpose: that is what teaches the
			// parser the book names of the languages it holds, so a reference
			// written the way the user's own Bible writes it resolves.
			st, err := a.store()
			if err != nil {
				return err
			}
			ranges, err := bible.Parse(args[0])
			if err != nil {
				return err
			}
			if !st.HasBible() {
				if a.offline {
					return fmt.Errorf("no study Bible in the library: pubkit sync nwtsty")
				}
				a.logf("no study Bible in the library (nwtsty, ~127 MB); syncing it")
				if _, err := a.syncOne("nwtsty", "", false); err != nil {
					return err
				}
			}
			res := verseResult{Ref: bible.FormatList(ranges), Ranges: ranges, Translation: st.BibleTitle()}
			for _, r := range ranges {
				vs, err := st.Verses(r.FirstID(), r.LastID())
				if err != nil {
					return err
				}
				if len(vs) == 0 {
					return fmt.Errorf("%s is not in the library's Bible", r.Long())
				}
				for i := range vs {
					for j := range vs[i].XRefs {
						x := &vs[i].XRefs[j]
						var rs []bible.Range
						for _, t := range x.Targets {
							if rg, ok := bible.FromIDs(t[0], t[1]); ok {
								rs = append(rs, rg)
							}
						}
						x.Refs = strings.Split(bible.FormatList(rs), "; ")
					}
				}
				res.Verses = append(res.Verses, vs...)
				if !noCites {
					cites, _, err := st.CitedBy(r.FirstID(), r.LastID(), 0)
					if err != nil {
						return err
					}
					res.CitedBy = mergeCitations(res.CitedBy, cites)
				}
			}
			res.CitedTotal = len(res.CitedBy)
			if citations > 0 && len(res.CitedBy) > citations {
				res.CitedBy = res.CitedBy[:citations]
			}
			if noNotes {
				for i := range res.Verses {
					res.Verses[i].Notes = nil
				}
			}
			if a.jsonOut {
				if res.CitedBy == nil {
					res.CitedBy = []store.Citation{}
				}
				return a.printJSON(res)
			}
			a.printVerses(res, ranges)
			return nil
		},
	}
	cmd.Flags().IntVar(&citations, "citations", 25, "how many citing documents to show (0 = all)")
	cmd.Flags().BoolVar(&noNotes, "no-notes", false, "leave out the study notes")
	cmd.Flags().BoolVar(&noCites, "no-citations", false, "leave out the documents citing the passage")
	return cmd
}

// verseText loads a passage as plain text, for anything that needs the words
// rather than the structure.
func (a *app) verseText(reference string) (text, source string, err error) {
	st, err := a.store()
	if err != nil {
		return "", "", err
	}
	ranges, err := bible.Parse(reference)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	for _, r := range ranges {
		vs, err := st.Verses(r.FirstID(), r.LastID())
		if err != nil {
			return "", "", err
		}
		if len(vs) == 0 {
			return "", "", fmt.Errorf("%s is not in the library's Bible", r.Long())
		}
		for _, v := range vs {
			b.WriteString(v.Text)
			b.WriteByte(' ')
		}
	}
	return b.String(), bible.FormatList(ranges), nil
}

func mergeCitations(have, add []store.Citation) []store.Citation {
	idx := map[int]int{}
	for i, c := range have {
		idx[c.DocID] = i
	}
	for _, c := range add {
		if i, ok := idx[c.DocID]; ok {
			for _, p := range c.PIDs {
				if !containsInt(have[i].PIDs, p) {
					have[i].PIDs = append(have[i].PIDs, p)
				}
			}
			continue
		}
		idx[c.DocID] = len(have)
		have = append(have, c)
	}
	return have
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (a *app) printVerses(res verseResult, ranges []bible.Range) {
	p := a.printf
	var heads []string
	for _, r := range ranges {
		heads = append(heads, r.Long())
	}
	if res.Translation != "" {
		p("%s · %s\n\n", strings.Join(heads, "; "), res.Translation)
	} else {
		p("%s\n\n", strings.Join(heads, "; "))
	}
	multiChapter := false
	for _, v := range res.Verses {
		if v.Chapter != res.Verses[0].Chapter || v.Book != res.Verses[0].Book {
			multiChapter = true
		}
	}
	label := func(v store.Verse) string {
		if v.Verse == 0 {
			return "(heading)"
		}
		if multiChapter || len(res.Verses) == 1 {
			bk, _ := bible.BookByNum(v.Book)
			return fmt.Sprintf("%s %d:%d", bk.Short, v.Chapter, v.Verse)
		}
		return fmt.Sprint(v.Verse)
	}
	for _, v := range res.Verses {
		p("%s %s\n", label(v), v.Text)
	}
	var fns, xrefs, notes []string
	for _, v := range res.Verses {
		cv := fmt.Sprintf("%d:%d", v.Chapter, v.Verse)
		for _, f := range v.Footnotes {
			fns = append(fns, fmt.Sprintf("  %s %s «%s»: %s", cv, f.Marker, f.Anchor, f.Text))
		}
		for _, x := range v.XRefs {
			xrefs = append(xrefs, fmt.Sprintf("  %s %s «%s» → %s", cv, x.Marker, x.Anchor, strings.Join(x.Refs, "; ")))
		}
		for _, n := range v.Notes {
			notes = append(notes, fmt.Sprintf("  %s %s", n.Label, n.Text))
			for _, d := range n.Defines {
				if d.Text != "" {
					notes = append(notes, fmt.Sprintf("      %s: %s", d.Term, d.Text))
					continue
				}
				notes = append(notes, fmt.Sprintf("      %s → %s", d.Term, d.URL))
			}
		}
	}
	section := func(title string, lines []string, empty string) {
		p("\n%s\n", title)
		if len(lines) == 0 {
			p("  %s\n", empty)
			return
		}
		for _, l := range lines {
			p("%s\n", l)
		}
	}
	section("Footnotes", fns, "(none)")
	section("Marginal references", xrefs, "(none)")
	section("Study notes", notes, "(this Bible carries no notes for the passage)")
	if res.CitedTotal > 0 || len(res.CitedBy) > 0 {
		p("\nCited in %d documents in the library", res.CitedTotal)
		if len(res.CitedBy) < res.CitedTotal {
			p(" (first %d; use --citations 0 for all)", len(res.CitedBy))
		}
		p(":\n")
		for _, c := range res.CitedBy {
			var ps []string
			for _, pid := range c.PIDs {
				ps = append(ps, fmt.Sprint(pid))
			}
			p("  %-11d %-7s %s", c.DocID, c.Pub, c.Title)
			if len(ps) > 0 {
				p(" · pid %s", strings.Join(ps, ", "))
			}
			p("\n")
		}
	}
}
