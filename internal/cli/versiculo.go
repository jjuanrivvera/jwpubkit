package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type verseResult struct {
	Ref         string           `json:"referencia"`
	Ranges      []bible.Range    `json:"rangos"`
	Verses      []store.Verse    `json:"versiculos"`
	CitedBy     []store.Citation `json:"citado_en"`
	CitedTotal  int              `json:"total_documentos_que_citan"`
	Translation string           `json:"traduccion"`
}

func (a *app) versiculoCmd() *cobra.Command {
	var citas int
	var noNotes, noCites bool
	cmd := &cobra.Command{
		Use:     `versiculo "<referencia>"`,
		Aliases: []string{"versículo", "v", "verse"},
		Short:   "Texto TNM textual (edición de estudio) con notas, referencias marginales y quién lo cita",
		Long: `Muestra el texto de la Traducción del Nuevo Mundo (edición de estudio, nwtsty) tal cual,
con sus notas al pie, referencias marginales, notas de estudio y los documentos de la
biblioteca que citan el pasaje (tabla BibleCitation de cada publicación sincronizada).

Acepta referencias en español: "Jer 38:6", "Jeremías 38:1-13", "1 Cor. 13:4-7",
"Sal 23", "Jer 38:28-39:2", "Jer 38:6; 39:1, 4-6".`,
		Example: `  pubkit versiculo "Jer 38:6"
  pubkit versiculo "Jer 38:1-13" --sin-notas
  pubkit versiculo "Juan 3:16" --citas 40 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ranges, err := bible.Parse(args[0])
			if err != nil {
				return err
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			if !st.HasBible() {
				if a.offline {
					return fmt.Errorf("la Biblia de estudio no está en la biblioteca: pubkit sync nwtsty")
				}
				a.logf("la Biblia de estudio (nwtsty, ~127 MB) no está en la biblioteca; sincronizando")
				if _, err := a.syncOne("nwtsty", "", false); err != nil {
					return err
				}
			}
			res := verseResult{Ref: bible.FormatList(ranges), Ranges: ranges, Translation: "Traducción del Nuevo Mundo (edición de estudio)"}
			for _, r := range ranges {
				vs, err := st.Verses(r.FirstID(), r.LastID())
				if err != nil {
					return err
				}
				if len(vs) == 0 {
					return fmt.Errorf("%s no está en la Biblia de la biblioteca", r.Long())
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
			if citas > 0 && len(res.CitedBy) > citas {
				res.CitedBy = res.CitedBy[:citas]
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
	cmd.Flags().IntVar(&citas, "citas", 25, "máximo de documentos que citan el pasaje a mostrar (0 = todos)")
	cmd.Flags().BoolVar(&noNotes, "sin-notas", false, "omitir las notas de estudio")
	cmd.Flags().BoolVar(&noCites, "sin-citas", false, "omitir los documentos que citan el pasaje")
	return cmd
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
	p("%s · %s\n\n", strings.Join(heads, "; "), res.Translation)
	multiChapter := false
	for _, v := range res.Verses {
		if v.Chapter != res.Verses[0].Chapter || v.Book != res.Verses[0].Book {
			multiChapter = true
		}
	}
	label := func(v store.Verse) string {
		if v.Verse == 0 {
			return "(encabezado)"
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
	section("Notas al pie", fns, "(ninguna)")
	section("Referencias marginales", xrefs, "(ninguna)")
	section("Notas de estudio", notes, "(la edición de estudio no trae notas para este pasaje)")
	if res.CitedTotal > 0 || len(res.CitedBy) > 0 {
		p("\nCitado en %d documentos de la biblioteca", res.CitedTotal)
		if len(res.CitedBy) < res.CitedTotal {
			p(" (primeros %d; usa --citas 0 para todos)", len(res.CitedBy))
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
