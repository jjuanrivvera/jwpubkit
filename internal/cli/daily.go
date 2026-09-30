package cli

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type dailyOut struct {
	Date      string `json:"date"`
	DocID     int    `json:"docid"`
	Theme     string `json:"theme_text"`
	Reference string `json:"theme_reference"`
	Comment   string `json:"comment"`
	Citation  string `json:"citation"`
	URL       string `json:"url"`
}

func commandDate(args []string) (time.Time, error) {
	if len(args) == 0 {
		return time.Now(), nil
	}
	day, err := time.ParseInLocation("2006-01-02", args[0], time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (use YYYY-MM-DD)", args[0])
	}
	return day, nil
}

func (a *app) commandStore(cmd *cobra.Command) (*store.Store, error) {
	a.ctx = cmd.Context()
	st, err := a.store()
	if err != nil {
		return nil, err
	}
	a.st = st.WithContext(cmd.Context())
	return a.st, nil
}

func (a *app) dailyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "daily [YYYY-MM-DD]", Aliases: []string{"texto"},
		Short: "Read a day's text from the indexed yearly examination volume",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			day, err := commandDate(args)
			if err != nil {
				return err
			}
			st, err := a.commandStore(cmd)
			if err != nil {
				return err
			}
			out, err := readDaily(st, day)
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(out)
			}
			a.printf("%s\n%s\nReference: %s\n\n%s\n\nCitation: %s\n%s\n", out.Date, out.Theme, out.Reference, out.Comment, out.Citation, out.URL)
			return nil
		},
	}
}

func readDaily(st *store.Store, day time.Time) (*dailyOut, error) {
	dated, err := st.DatedDocs("es", meeting.DateNum(day))
	if err != nil {
		return nil, err
	}
	for _, dd := range dated {
		d, err := st.Doc(dd.DocID)
		if err != nil {
			return nil, err
		}
		if d.Class != 4 {
			continue
		}
		parsed, err := content.Parse(d.HTML)
		if err != nil {
			return nil, err
		}
		blocks := dailyBlocks(parsed, dd, day)
		out := &dailyOut{Date: day.Format("2006-01-02"), DocID: d.DocID}
		var comment []string
		for _, blk := range blocks {
			if blk.Kind == content.KindHeading || blk.Kind == content.KindContext {
				continue
			}
			if out.Theme == "" && len(blk.BibleRefs()) > 0 {
				out.Theme = blk.Text()
				out.Reference = bible.FormatList(blk.BibleRefs())
				out.URL = content.DocURL(d.DocID, blk.PID)
				continue
			}
			if out.Theme != "" && blk.Text() != "" {
				comment = append(comment, blk.Text())
				for _, link := range blk.PubLinks() {
					out.Citation = link.Text
				}
			}
		}
		if out.Theme == "" {
			continue
		}
		out.Comment = strings.Join(comment, "\n\n")
		if out.Citation == "" {
			location, _, _ := strings.Cut(dd.Caption, " · ")
			out.Citation = content.Cite(location, d.Pub.MepsSymbol, d.Pub.Issue, 0, 0, d.DocID, 0).Text
		}
		return out, nil
	}
	// The volume's symbol carries the year and takes no issue: "es26", not
	// "es --issue 2026", which the CDN answers with a 400.
	return nil, fmt.Errorf("no daily entry for %s in the library (pubkit sync es%02d)",
		day.Format("2006-01-02"), day.Year()%100)
}

func dailyBlocks(parsed *content.Doc, dd store.DatedDoc, day time.Time) []*content.Block {
	if link := dailyLink(dd.Link); link != nil && link.First > 0 && dd.First == dd.Last && dd.First == meeting.DateNum(day) {
		last := link.Last
		var out []*content.Block
		level := 0
		for _, blk := range parsed.Blocks {
			if blk.PID == link.First && blk.Kind == content.KindHeading {
				level = blk.Level
			}
			if len(out) > 0 && last <= link.First && blk.Kind == content.KindHeading && level > 0 && blk.Level <= level {
				break
			}
			if blk.PID >= link.First && (last <= link.First || blk.PID <= last) {
				out = append(out, blk)
			}
		}
		return out
	}
	// Monthly containers have one heading per day. Counting those headings
	// avoids interpreting the translated date printed on each one.
	entry := 0
	var out []*content.Block
	for _, blk := range parsed.Blocks {
		if blk.Kind == content.KindHeading && blk.Level == 2 {
			entry++
		}
		if entry == day.Day() {
			out = append(out, blk)
		}
	}
	return out
}

func dailyLink(raw string) *content.Link {
	raw = strings.TrimPrefix(raw, "jwpub://")
	parsed, _ := content.Parse(`<p data-pid="1"><a href="jwpub://` + html.EscapeString(raw) + `">entry</a></p>`)
	if parsed != nil && len(parsed.Blocks) > 0 {
		links := parsed.Blocks[0].PubLinks()
		if len(links) > 0 {
			return links[0]
		}
	}
	return nil
}
