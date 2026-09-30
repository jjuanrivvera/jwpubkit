package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type weekUpdateRow struct {
	DocID  int    `json:"docid,omitempty"`
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Symbol string `json:"symbol,omitempty"`
	Issue  string `json:"issue,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type weekUpdateOut struct {
	Monday string          `json:"monday"`
	DryRun bool            `json:"dry_run"`
	Rows   []weekUpdateRow `json:"results"`
	Notes  []string        `json:"notes"`
}

func (a *app) updateWeekCmd() *cobra.Command {
	var dryRun, withReferences bool
	var interval time.Duration
	cmd := &cobra.Command{
		Use: "update-week [YYYY-MM-DD]", Aliases: []string{"semanal"},
		Short: "Sync the week's workbook, study issues and referenced video subtitles",
		Long: `Checks publication checksums with the CDN and syncs changed copies. Fetches the
workbook's referenced video subtitles through the subtitles command's cache and
fallback logic. Video failures are reported without failing the whole update.
--dry-run reads an existing library only, without network or writes; videos can
only be listed when the week's workbook is already indexed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval < 0 {
				return fmt.Errorf("interval must not be negative")
			}
			day, err := commandDate(args)
			if err != nil {
				return err
			}
			a.ctx = cmd.Context()
			var st *store.Store
			if dryRun {
				st, err = store.OpenReadOnly(cmd.Context(), a.libDir, a.lang)
				if st != nil {
					defer st.Close()
				}
				if os.IsNotExist(err) {
					err = nil
				}
			} else {
				st, err = a.commandStore(cmd)
			}
			if err != nil {
				return err
			}
			out, updateErr := a.updateWeek(st, meeting.Monday(day), dryRun)
			if withReferences && updateErr == nil {
				updateErr = a.updateReferences(st, meeting.Monday(day), dryRun, interval, out)
			}
			if a.jsonOut {
				if err := a.printJSON(out); err != nil {
					return err
				}
			} else {
				for _, row := range out.Rows {
					a.printf("%s: %s", row.Key, row.Status)
					if row.Error != "" {
						a.printf(" (%s)", row.Error)
					}
					a.printf("\n")
				}
				for _, note := range out.Notes {
					a.printf("Note: %s\n", note)
				}
			}
			return updateErr
		},
	}
	cmd.Flags().BoolVar(&withReferences, "with-references", false, "also sync missing publications named by workbook extracts")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "pause between referenced publication requests")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report planned work without network or writes")
	return cmd
}

func (a *app) updateWeek(st *store.Store, monday time.Time, dryRun bool) (*weekUpdateOut, error) {
	out := &weekUpdateOut{Monday: monday.Format("2006-01-02"), DryRun: dryRun, Rows: []weekUpdateRow{}, Notes: []string{}}
	pubs := []weekUpdateRow{{Kind: "publication", Symbol: "mwb", Issue: meeting.WorkbookIssue(monday)}}
	for _, issue := range meeting.WatchtowerIssues(monday) {
		pubs = append(pubs, weekUpdateRow{Kind: "publication", Symbol: "w", Issue: issue})
	}
	failed := 0
	for _, row := range pubs {
		row.Key = store.PubKey(row.Symbol, a.lang, row.Issue)
		row.Status = "would check and sync if changed"
		if !dryRun {
			res, err := a.syncOne(row.Symbol, row.Issue, false)
			switch {
			case err != nil:
				row.Status, row.Error = "failed", err.Error()
				failed++
			case res.UpToDate:
				row.Status = "already current"
			default:
				row.Status = "synced"
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if st == nil {
		out.Notes = append(out.Notes, "Video keys are unknown until the workbook is synced.")
		return out, nil
	}
	builder := &meeting.Builder{Store: st}
	dd, err := builder.FindWorkbook(monday)
	if err != nil {
		return out, err
	}
	if dd == nil {
		out.Notes = append(out.Notes, "No indexed workbook entry for this week; video keys are unknown.")
	} else {
		w := &meeting.Week{Monday: out.Monday}
		if err := builder.BuildWorkbook(w, dd.DocID, *dd); err != nil {
			return out, err
		}
		for _, key := range w.SortedVideoKeys() {
			row := weekUpdateRow{Kind: "video_subtitles", Key: key, Status: "would fetch subtitles"}
			cached, err := st.Video(key, a.lang)
			if err != nil {
				return out, err
			}
			path := filepath.Join(st.Dir, subtitleDir, key+"."+a.lang+".vtt")
			if cached != nil {
				if _, err := readCachedVTT(st.Dir, path); err == nil {
					row.Status = "already current"
				}
			}
			if !dryRun {
				// Use exactly the same cache, mediator fallback and cue indexing as
				// subtitles, while reserving stdout for the update report.
				subApp := *a
				subApp.out, subApp.jsonOut = io.Discard, false
				subcmd := subApp.subtitlesCmd()
				subcmd.SetOut(io.Discard)
				subcmd.SetErr(a.err)
				subcmd.SilenceErrors, subcmd.SilenceUsage = true, true
				subcmd.SetArgs([]string{key})
				if err := subcmd.ExecuteContext(a.ctx); err != nil {
					row.Status, row.Error = "failed", err.Error()
				} else if row.Status != "already current" {
					row.Status = "synced"
				}
			}
			out.Rows = append(out.Rows, row)
		}
	}
	if failed > 0 {
		return out, fmt.Errorf("%d of %d publications failed", failed, len(pubs))
	}
	return out, a.ctx.Err()
}
