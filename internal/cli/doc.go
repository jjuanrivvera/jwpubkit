package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type docOut struct {
	DocID     int                `json:"docid"`
	Pub       string             `json:"publicacion"`
	PubKey    string             `json:"clave"`
	Title     string             `json:"titulo"`
	Context   string             `json:"contexto,omitempty"`
	URL       string             `json:"url"`
	Partial   string             `json:"parcial,omitempty"`
	Blocks    []blockOut         `json:"parrafos"`
	Images    []*content.Image   `json:"imagenes"`
	Videos    []content.VideoRef `json:"videos"`
	BibleRefs []string           `json:"citas_biblicas"`
	PubRefs   []refOut           `json:"referencias"`
}

type blockOut struct {
	PID      int      `json:"pid"`
	Kind     string   `json:"tipo"`
	Level    int      `json:"nivel,omitempty"`
	Num      int      `json:"numero,omitempty"`
	NumLabel string   `json:"numero_etiqueta,omitempty"`
	Sub      int      `json:"subentrada,omitempty"`
	Text     string   `json:"texto"`
	Question bool     `json:"pregunta,omitempty"`
	Box      bool     `json:"recuadro,omitempty"`
	RelPID   int      `json:"pregunta_pid,omitempty"`
	Bible    []string `json:"citas,omitempty"`
	Refs     []refOut `json:"referencias,omitempty"`
	Videos   []string `json:"videos,omitempty"`
}

type refOut struct {
	Text  string `json:"texto"`
	DocID int    `json:"docid"`
	Pars  string `json:"parrafos,omitempty"`
	PID   int    `json:"pid"`
	URL   string `json:"url"`
}

func (a *app) docCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "doc <docid>",
		Short: "Un documento completo en Markdown limpio, JSON o texto",
		Long: `Muestra un documento de la biblioteca por su docid (el mismo número de wol:
wol.jw.org/es/wol/d/r4/lp-s/<docid>) con párrafos numerados como los citan las
publicaciones, preguntas de estudio, citas bíblicas, referencias a otras publicaciones
(con su docid) e imágenes (sus archivos se bajan con "pubkit imagen <docid>").

Si el documento no está sincronizado pero otra publicación trae un extracto de él
(la Guía trae el capítulo del libro de estudio, por ejemplo), muestra ese extracto.`,
		Example: `  pubkit doc 2026485
  pubkit doc 1200001265 --formato txt
  pubkit doc 202026255 --formato json | jq '.parrafos[] | select(.pregunta)'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			docid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("docid inválido %q", args[0])
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			out, parsed, err := a.loadDoc(st, docid)
			if err != nil {
				return err
			}
			if a.jsonOut {
				format = "json"
			}
			switch format {
			case "json":
				return a.printJSON(out)
			case "txt", "texto":
				a.printf("%s\n%s · docid %d · %s\n", out.Title, out.Pub, out.DocID, out.URL)
				if out.Partial != "" {
					a.printf("[PARCIAL] %s\n", out.Partial)
				}
				a.printf("\n%s", parsed.PlainText())
			case "md", "markdown":
				a.printf("---\ndocid: %d\npublicacion: %s\ntitulo: %q\nurl: %s\n", out.DocID, out.Pub, out.Title, out.URL)
				if out.Partial != "" {
					a.printf("parcial: %q\n", out.Partial)
				}
				a.printf("---\n\n%s", parsed.Markdown(content.RenderOptions{DocID: docid}))
				a.printReferences(out)
			default:
				return fmt.Errorf("formato %q desconocido (md, json o txt)", format)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "formato", "f", "md", "md, json o txt")
	return cmd
}

// loadDoc finds a document in the library or, failing that, the largest
// extract of it that another publication carries.
func (a *app) loadDoc(st *store.Store, docid int) (*docOut, *content.Doc, error) {
	out := &docOut{DocID: docid, URL: content.WolDocURL(docid, 0)}
	var htmlText string
	d, err := st.Doc(docid)
	switch {
	case err == nil:
		htmlText = d.HTML
		out.Pub, out.PubKey, out.Title, out.Context = d.Pub.MepsSymbol, d.Pub.Key, d.Title, d.ContextTitle
		if htmlText == "" {
			return nil, nil, fmt.Errorf("el documento %d (%s) no tiene contenido propio (en una Biblia el texto está en los versículos: usa pubkit versiculo)", docid, d.Title)
		}
	case errors.Is(err, store.ErrNoDoc):
		exts, xerr := st.ExtractsFor(docid)
		if xerr != nil {
			return nil, nil, xerr
		}
		if len(exts) == 0 {
			return nil, nil, fmt.Errorf("el documento %d no está en la biblioteca ni hay extractos de él; sincroniza su publicación (pubkit pubs para ver lo que hay)", docid)
		}
		e := exts[0]
		htmlText = e.HTML
		out.Pub, out.Title = e.RefSymbol, e.Title
		out.Partial = fmt.Sprintf("extracto de %s que trae %s (párrafos %d-%d); el documento completo: %s", e.Caption, e.PubFile[strings.LastIndex(e.PubFile, "/")+1:], e.RefBegin, e.RefEnd, syncCommand(e))
		if e.RefBegin == 0 {
			out.Partial = fmt.Sprintf("extracto de %s que trae %s; el documento completo: %s", e.Caption, e.PubFile[strings.LastIndex(e.PubFile, "/")+1:], syncCommand(e))
		}
	default:
		return nil, nil, err
	}
	parsed, err := content.Parse(htmlText)
	if err != nil {
		return nil, nil, err
	}
	if out.Title == "" {
		for _, b := range parsed.Blocks {
			if b.Kind == content.KindHeading && b.Level == 1 {
				out.Title = b.Text()
				break
			}
		}
	}
	seenBible := map[string]bool{}
	for _, b := range parsed.Blocks {
		bo := blockOut{PID: b.PID, Kind: b.Kind, Level: b.Level, Num: b.Num, NumLabel: b.NumLabel, Sub: b.Sub, Text: b.Text(),
			Question: b.IsQuestion(), Box: b.InBox, RelPID: b.RelPID}
		if bo.Text == "" {
			continue
		}
		for _, r := range b.BibleRefs() {
			bo.Bible = append(bo.Bible, r.String())
			if !seenBible[r.String()] {
				seenBible[r.String()] = true
				out.BibleRefs = append(out.BibleRefs, r.String())
			}
		}
		for _, l := range b.PubLinks() {
			if l.DocID == 0 || l.DocID == docid {
				continue
			}
			r := refOut{Text: l.Text, DocID: l.DocID, Pars: l.Pars, PID: b.PID, URL: content.WolDocURL(l.DocID, l.First)}
			bo.Refs = append(bo.Refs, r)
			out.PubRefs = append(out.PubRefs, r)
		}
		for _, v := range b.Videos {
			bo.Videos = append(bo.Videos, v.Key)
		}
		out.Blocks = append(out.Blocks, bo)
	}
	out.Images = parsed.Images
	out.Videos = parsed.Videos
	if out.Images == nil {
		out.Images = []*content.Image{}
	}
	if out.Videos == nil {
		out.Videos = []content.VideoRef{}
	}
	if out.BibleRefs == nil {
		out.BibleRefs = []string{}
	}
	if out.PubRefs == nil {
		out.PubRefs = []refOut{}
	}
	return out, parsed, nil
}

func syncCommand(e store.Extract) string {
	sym := e.RefUndated
	if sym == "" {
		return "pubkit sync <símbolo>"
	}
	if e.RefIssue != 0 {
		return fmt.Sprintf("pubkit sync %s --issue %s", sym, store.NormalizeIssue(strconv.Itoa(e.RefIssue)))
	}
	return "pubkit sync " + sym
}

func (a *app) printReferences(out *docOut) {
	if len(out.BibleRefs) == 0 && len(out.PubRefs) == 0 && len(out.Videos) == 0 {
		return
	}
	a.printf("\n---\n\n## Referencias\n\n")
	if len(out.BibleRefs) > 0 {
		a.printf("**Citas bíblicas:** %s\n\n", strings.Join(out.BibleRefs, "; "))
	}
	if len(out.PubRefs) > 0 {
		a.printf("**Publicaciones:**\n\n")
		for _, r := range out.PubRefs {
			a.printf("- %s → docid %d", r.Text, r.DocID)
			if r.Pars != "" {
				a.printf(" ¶%s", r.Pars)
			}
			a.printf(" (en pid %d)\n", r.PID)
		}
		a.printf("\n")
	}
	if len(out.Videos) > 0 {
		a.printf("**Videos:**\n\n")
		for _, v := range out.Videos {
			a.printf("- `%s` %s\n", v.Key, v.Title)
		}
	}
}
