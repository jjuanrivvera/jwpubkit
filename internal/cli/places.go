package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type placesOut struct {
	Reference string              `json:"reference"`
	Sources   []store.PlaceSource `json:"sources"`
	Notes     []string            `json:"notes"`
}

func (a *app) placesCmd() *cobra.Command {
	var kind, figure string
	cmd := &cobra.Command{
		Use: "places <passage>", Aliases: []string{"lugares"},
		Short: "Chapter note terms, citing atlas maps and Bible appendix figures",
		Long: `Accepts chapter and verse ranges, and classifies evidence by source.
Encyclopedia terms remain unclassified unless publication-title evidence exists.
Reports note terms with encyclopedia articles, atlas documents citing the chapter, and library-wide study Bible
appendix figures with their list-item labels. No source links a place name to a
position on a map. Reads only the library; nothing is synced.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.commandStore(cmd)
			if err != nil {
				return err
			}
			rs, err := bible.Parse(args[0])
			if err != nil {
				return err
			}

			out := &placesOut{Reference: bible.FormatList(rs), Sources: []store.PlaceSource{}, Notes: []string{}}
			seen := map[string]bool{}
			for _, r := range rs {
				part, err := a.chapterPlaces(st, r)
				if err != nil {
					return err
				}
				if len(out.Notes) == 0 {
					out.Notes = part.Notes
				}
				for _, row := range part.Sources {
					key := fmt.Sprintf("%s:%s:%d", row.Kind, row.Key, row.DocID)
					if seen[key] {
						continue
					}
					seen[key] = true
					switch row.Kind {
					case "atlas_map":
						row.Classification = "map"
						row.ClassificationBasis = "atlas publication and overlapping Bible citation"
					case "appendix_figure":
						row.Classification = "appendix_figure"
						row.ClassificationBasis = "Bible appendix document class and SVG multimedia"
					default:
						row.Classification = "unclassified"
						row.ClassificationBasis = "exact-title encyclopedia match is not sufficient to identify a place"
						pubs, err := st.Pubs()
						if err != nil {
							return err
						}
						for _, pub := range pubs {
							if len([]rune(row.Title)) > 3 && strings.Contains(strings.ToLower(pub.Title), strings.ToLower(row.Title)) {
								row.Classification = "publication_title"
								row.ClassificationBasis = "term also occurs in an indexed publication title"
								break
							}
						}
					}
					if kind != "" && row.Classification != kind {
						continue
					}
					if figure != "" {
						images := []store.PlaceFigure{}
						for _, img := range row.Images {
							if img.File == figure {
								images = append(images, img)
							}
						}
						if len(images) == 0 {
							continue
						}
						row.Images = images
					}
					out.Sources = append(out.Sources, row)
				}
			}

			if a.jsonOut {
				return a.printJSON(out)
			}
			a.printf("%s\n", out.Reference)
			for _, row := range out.Sources {
				a.printf("\n%s: %s · %s · docid %d\nSource: %s\n%s\n", row.Kind, row.Title, row.Publication, row.DocID, row.Source, row.URL)
				if len(row.Names) > 0 {
					a.printf("Labels: %s\n", strings.Join(row.Names, "; "))
				}
				for _, img := range row.Images {
					a.printf("Image: %s · %s\n", img.File, img.Caption)
				}
			}
			for _, note := range out.Notes {
				a.printf("Note: %s\n", note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "classification filter: map, appendix_figure, publication_title or unclassified")
	cmd.Flags().StringVar(&figure, "figure", "", "select an exact appendix or atlas figure filename")
	return cmd
}

func (a *app) chapterPlaces(st *store.Store, r bible.Range) (*placesOut, error) {
	out := &placesOut{Reference: r.Long(), Sources: []store.PlaceSource{}, Notes: []string{
		"Encyclopedia terms use the dossier's exact-title heuristic; they are not classified as places and the list can miss terms.",
		"Atlas documents cite the chapter; no data links these terms to locations on those maps.",
		"Appendix figures and their list-item labels are library-wide, with no chapter or label-to-map-position link.",
	}}
	verses, err := st.Verses(r.FirstID(), r.LastID())
	if err != nil {
		return nil, err
	}
	if len(verses) == 0 {
		out.Notes = append(out.Notes, "The chapter's Bible text and notes are not in the library.")
	}
	if !st.HasSymbol("it") {
		out.Notes = append(out.Notes, "The encyclopedia is not in the library (pubkit sync it).")
	} else {
		for _, term := range a.chapterTerms(st, verses) {
			if !term.InIt {
				continue
			}
			d, err := st.Doc(term.ItDocID)
			if err != nil {
				return nil, err
			}
			out.Sources = append(out.Sources, store.PlaceSource{Kind: "encyclopedia_term", Source: "verse_note/verse_fn -> chapterTerms -> doc.title (it)",
				DocID: d.DocID, Title: d.Title, Publication: "it", Key: d.Pub.Key, URL: content.DocURL(d.DocID, 0), Names: []string{term.Name}, Images: []store.PlaceFigure{}})
		}
	}
	docs, err := st.PlaceDocuments(r.FirstID(), r.LastID())
	if err != nil {
		return nil, err
	}
	out.Sources = append(out.Sources, docs...)
	return out, nil
}
