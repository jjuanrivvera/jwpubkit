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
	Pub       string             `json:"publication"`
	PubKey    string             `json:"key"`
	Title     string             `json:"title"`
	Context   string             `json:"context,omitempty"`
	URL       string             `json:"url"`
	Partial   string             `json:"partial,omitempty"`
	Blocks    []blockOut         `json:"paragraphs"`
	Images    []*content.Image   `json:"images"`
	Videos    []content.VideoRef `json:"videos"`
	BibleRefs []string           `json:"bible_citations"`
	PubRefs   []refOut           `json:"references"`
}

type blockOut struct {
	PID      int      `json:"pid"`
	Kind     string   `json:"kind"`
	Level    int      `json:"level,omitempty"`
	Num      int      `json:"number,omitempty"`
	NumLabel string   `json:"number_label,omitempty"`
	Sub      int      `json:"subentry,omitempty"`
	Text     string   `json:"text"`
	Question bool     `json:"question,omitempty"`
	Box      bool     `json:"box,omitempty"`
	RelPID   int      `json:"question_pid,omitempty"`
	Bible    []string `json:"citations,omitempty"`
	Refs     []refOut `json:"references,omitempty"`
	Videos   []string `json:"videos,omitempty"`
}

type refOut struct {
	Text  string `json:"text"`
	DocID int    `json:"docid"`
	Pars  string `json:"paragraphs,omitempty"`
	PID   int    `json:"pid"`
	URL   string `json:"url"`
}

func (a *app) docCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "doc <docid>",
		Short: "A whole document as clean Markdown, JSON or plain text",
		Long: `Prints a document from the library by its docid — the same number wol uses — with
its paragraphs numbered the way publications cite them, study questions, Bible
citations, references to other publications (with their docid) and images (the files
themselves come down with "pubkit image <docid>").

When the document is not synced but another publication carries an extract of it, the
extract is printed instead.`,
		Example: `  pubkit doc 2026485
  pubkit doc 1200001265 --format txt
  pubkit doc 202026255 --format json | jq '.paragraphs[] | select(.question)'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			docid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid docid %q", args[0])
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
			case "txt", "text":
				a.printf("%s\n%s · docid %d · %s\n", out.Title, out.Pub, out.DocID, out.URL)
				if out.Partial != "" {
					a.printf("[PARTIAL] %s\n", out.Partial)
				}
				a.printf("\n%s", parsed.PlainText())
			case "md", "markdown":
				a.printf("---\ndocid: %d\npublication: %s\ntitle: %q\nurl: %s\n", out.DocID, out.Pub, out.Title, out.URL)
				if out.Partial != "" {
					a.printf("partial: %q\n", out.Partial)
				}
				a.printf("---\n\n%s", parsed.Markdown(content.RenderOptions{DocID: docid}))
				a.printReferences(out)
			default:
				return fmt.Errorf("unknown format %q (md, json or txt)", format)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "md", "md, json or txt")
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
			return nil, nil, fmt.Errorf("document %d (%s) has no content of its own (in a Bible the text lives in the verses: use pubkit verse)", docid, d.Title)
		}
	case errors.Is(err, store.ErrNoDoc):
		exts, xerr := st.ExtractsFor(docid)
		if xerr != nil {
			return nil, nil, xerr
		}
		if len(exts) == 0 {
			return nil, nil, fmt.Errorf("document %d is not in the library and nothing extracts it; sync its publication (pubkit pubs shows what you have)", docid)
		}
		e := exts[0]
		htmlText = e.HTML
		out.Pub, out.Title = e.RefSymbol, e.Title
		out.Partial = fmt.Sprintf("extract of %s carried by %s (paragraphs %d-%d); the whole document: %s", e.Caption, e.PubFile[strings.LastIndex(e.PubFile, "/")+1:], e.RefBegin, e.RefEnd, syncCommand(e))
		if e.RefBegin == 0 {
			out.Partial = fmt.Sprintf("extract of %s carried by %s; the whole document: %s", e.Caption, e.PubFile[strings.LastIndex(e.PubFile, "/")+1:], syncCommand(e))
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
		return "pubkit sync <symbol>"
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
	a.printf("\n---\n\n## References\n\n")
	if len(out.BibleRefs) > 0 {
		a.printf("**Bible citations:** %s\n\n", strings.Join(out.BibleRefs, "; "))
	}
	if len(out.PubRefs) > 0 {
		a.printf("**Publications:**\n\n")
		for _, r := range out.PubRefs {
			a.printf("- %s → docid %d", r.Text, r.DocID)
			if r.Pars != "" {
				a.printf(" ¶%s", r.Pars)
			}
			a.printf(" (at pid %d)\n", r.PID)
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
