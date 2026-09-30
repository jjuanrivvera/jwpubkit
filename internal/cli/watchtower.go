package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
)

func (a *app) watchtowerCmd() *cobra.Command {
	return &cobra.Command{
		Use: "watchtower [YYYY-MM-DD]", Aliases: []string{"atalaya"},
		Short: "Read the week's study article, with each paragraph's question and scriptures",
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
			monday := meeting.Monday(day)
			builder := &meeting.Builder{Store: st}
			docid, err := builder.FindWatchtower(monday)
			if err != nil {
				return err
			}
			if docid == 0 {
				return fmt.Errorf("no study Watchtower for the week of %s in the library; sync w issues %s (pubkit update-week %s)", monday.Format("2006-01-02"), strings.Join(meeting.WatchtowerIssues(monday), ", "), monday.Format("2006-01-02"))
			}
			out, err := builder.BuildWatchtowerArticle(docid)
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(out)
			}
			a.printf("%s\n%s · docid %d\n%s\nTheme: %s\nReference: %s\n", out.Title, out.Date, out.DocID, out.URL, out.Theme, out.ThemeRef)
			for _, song := range out.Songs {
				a.printf("Song (%s): %d %s\n", song.When, song.Number, song.Title)
			}
			for _, p := range out.Paragraphs {
				a.printf("\nParagraph %d (pid %d)\nQuestion: %s\n%s\n", p.Number, p.PID, p.Question, p.Text)
				if len(p.Scriptures) > 0 {
					a.printf("Scriptures: %s\n", strings.Join(p.Scriptures, "; "))
				}
				for _, img := range p.Images {
					a.printf("Image: %s · %s\n", img.File, img.Caption)
				}
			}
			for _, box := range out.Boxes {
				a.printf("\nBox: %s\n", box.Title)
				for _, q := range box.Questions {
					a.printf("  %s\n", q)
				}
			}
			for _, img := range out.Images {
				a.printf("Image: %s · %s\n", img.File, img.Caption)
			}
			return nil
		},
	}
}
