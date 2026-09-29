package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

func (a *app) weekCmd() *cobra.Command {
	var noWT, extracts bool
	cmd := &cobra.Command{
		Use:     "week [YYYY-MM-DD]",
		Aliases: []string{"semana"},
		Short:   "The midweek meeting and that week's study Watchtower",
		Long: `Takes the Monday of the given date's week (today when none is given) and assembles,
from the meeting workbook: docid, the weekly Bible reading, the student reading with
its teaching lesson, songs, every part with its title, time, questions, references
(citations and docids, with the text the workbook carries for each one), videos (with
the key for "pubkit subtitles") and images. The congregation Bible study brings the
whole chapter the workbook points at: title, accounts, questions and videos.

When it exists, the week's study Watchtower is added too: docid, title, theme text,
songs and the questions paragraph by paragraph. Anything missing from the library is
synced first, unless --offline says otherwise.`,
		Example: `  pubkit week 2026-09-28
  pubkit week 2026-09-30 --extracts
  pubkit week --json 2026-09-28 | jq '.sections[].parts[] | {number, title, minutes}'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			day := time.Now()
			if len(args) == 1 {
				d, err := time.ParseInLocation("2006-01-02", args[0], time.Local)
				if err != nil {
					return fmt.Errorf("invalid date %q (use YYYY-MM-DD)", args[0])
				}
				day = d
			}
			w, err := a.buildWeek(day, !noWT)
			if err != nil {
				return err
			}
			if a.jsonOut {
				w.Normalize()
				return a.printJSON(w) // always with extracts: they save the trips to wol
			}
			a.printWeek(w, extracts)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noWT, "no-watchtower", false, "leave out the study Watchtower")
	cmd.Flags().BoolVar(&extracts, "extracts", false, "print the text the workbook carries for each reference")
	return cmd
}

func (a *app) buildWeek(day time.Time, withWT bool) (*meeting.Week, error) {
	st, err := a.store()
	if err != nil {
		return nil, err
	}
	monday := meeting.Monday(day)
	w := &meeting.Week{Monday: monday.Format("2006-01-02")}
	b := &meeting.Builder{Store: st}

	dd, err := b.FindWorkbook(monday)
	if err != nil {
		return nil, err
	}
	if dd == nil {
		issue := meeting.WorkbookIssue(monday)
		if a.offline {
			return nil, fmt.Errorf("the workbook for the week of %s is not in the library (pubkit sync mwb --issue %s)", w.Monday, issue)
		}
		a.logf("workbook mwb %s is not in the library; syncing it", issue)
		if _, err := a.syncOne("mwb", issue, false); err != nil {
			return nil, fmt.Errorf("syncing workbook mwb %s: %w", issue, err)
		}
		if dd, err = b.FindWorkbook(monday); err != nil {
			return nil, err
		}
		if dd == nil {
			return nil, fmt.Errorf("workbook mwb %s has no week of %s (an assembly, the Memorial or a visit?)", issue, w.Monday)
		}
	}
	if err := b.BuildWorkbook(w, dd.DocID, *dd); err != nil {
		return nil, err
	}

	if withWT {
		docid, err := b.FindWatchtower(monday)
		if err != nil {
			return nil, err
		}
		if docid == 0 && !a.offline {
			for _, issue := range meeting.WatchtowerIssues(monday) {
				if p, _ := st.PubByKey(store.PubKey("w", a.lang, issue)); p != nil {
					continue
				}
				a.logf("looking for the week's study Watchtower in w %s", issue)
				if _, err := a.syncOne("w", issue, false); err != nil {
					a.logf("w %s: %v", issue, err)
					continue
				}
				if docid, err = b.FindWatchtower(monday); err != nil {
					return nil, err
				}
				if docid != 0 {
					break
				}
			}
		}
		if docid != 0 {
			if err := b.BuildWatchtower(w, docid); err != nil {
				return nil, err
			}
		} else {
			w.Notes = append(w.Notes, "No study Watchtower found for this week (tried: w "+strings.Join(meeting.WatchtowerIssues(monday), ", ")+").")
		}
	}
	a.resolveVideos(w)
	return w, nil
}

// resolveVideos fills titles and durations from the mediator API, cached in
// the library so the next run needs no network.
func (a *app) resolveVideos(w *meeting.Week) {
	st, _ := a.store()
	for _, key := range w.SortedVideoKeys() {
		title, dur, err := a.videoInfo(st, key)
		if err != nil {
			if !errors.Is(err, errOffline) {
				w.Notes = append(w.Notes, fmt.Sprintf("Video %s: %v", key, err))
			}
			continue
		}
		w.SetVideoTitle(key, title, dur)
	}
}

func (a *app) videoInfo(st *store.Store, key string) (title, dur string, err error) {
	if v, _ := st.Video(key, a.lang); v != nil {
		return v.Title, subs.FormatTS(time.Duration(v.Duration * float64(time.Second))), nil
	}
	if a.offline {
		return "", "", errOffline
	}
	item, err := a.client().MediaItem(a.ctx, key)
	if err != nil {
		return "", "", err
	}
	_ = st.PutVideo(key, a.lang, item.Title, item.Duration, item.SubtitlesURL(), item)
	return item.Title, subs.FormatTS(time.Duration(item.Duration * float64(time.Second))), nil
}

func (a *app) printWeek(w *meeting.Week, extracts bool) {
	p := a.printf
	p("Week of %s (monday %s)\n", w.Range, w.Monday)
	if g := w.Workbook; g != nil {
		p("Meeting workbook: docid %d · %s · %s\n", g.DocID, g.Location, g.URL)
	}
	if r := w.WeeklyReading; r != nil {
		p("Weekly Bible reading: %s (%s)\n", r.Text, r.Ref)
	}
	if sr := w.StudentReading; sr != nil {
		line := "Student reading: " + sr.Ref
		if l := sr.Lesson; l != nil {
			line += fmt.Sprintf(" · %s «%s» (docid %d)", l.Text, l.Title, l.DocID)
		}
		p("%s\n", line)
	}
	if len(w.Songs) > 0 {
		var ss []string
		for _, s := range w.Songs {
			ss = append(ss, fmt.Sprintf("%d «%s» (%s)", s.Number, s.Title, s.When))
		}
		p("Songs: %s\n", strings.Join(ss, " · "))
	}
	for _, sec := range w.Sections {
		p("\n")
		if sec.Title != "" {
			p("%s\n", sec.Title)
		}
		for _, part := range sec.Parts {
			a.printPart(part, extracts)
		}
	}
	if len(w.Videos) > 0 {
		p("\nMeeting videos (transcript: pubkit subtitles <key>)\n")
		seen := map[string]bool{}
		for _, v := range w.Videos {
			if seen[v.Key] {
				continue
			}
			seen[v.Key] = true
			p("  %-28s %s", v.Key, v.Title)
			if v.Duration != "" {
				p(" (%s)", v.Duration)
			}
			if v.Part != "" {
				p(" · part %s", v.Part)
			}
			p("\n")
		}
	}
	if wt := w.Watchtower; wt != nil {
		p("\nSTUDY WATCHTOWER · %s\n", wt.Date)
		p("«%s» · docid %d · %s · %s\n", wt.Title, wt.DocID, wt.Location, wt.URL)
		if wt.Theme != "" {
			p("Theme text: %s\n", wt.Theme)
		}
		if wt.Summary != "" {
			p("Theme: %s\n", wt.Summary)
		}
		for _, s := range wt.Songs {
			p("Song %d «%s» (%s)\n", s.Number, s.Title, s.When)
		}
		p("Questions:\n")
		sub := ""
		for _, q := range wt.Questions {
			if q.Subheading != sub && q.Subheading != "" {
				p("  %s\n", q.Subheading)
				sub = q.Subheading
			}
			p("    %s. %s\n", q.Paragraphs, q.Text)
		}
		for _, box := range wt.Boxes {
			p("  Box «%s»:\n", box.Title)
			for _, q := range box.Questions {
				p("    - %s\n", q)
			}
		}
		if len(wt.Images) > 0 {
			p("  Images (pubkit image %d):\n", wt.DocID)
			for _, im := range wt.Images {
				p("    %s · %s\n", im.File, firstNonEmpty(im.Caption, im.Alt))
			}
		}
	}
	for _, n := range w.Notes {
		p("\nNote: %s\n", n)
	}
}

func (a *app) printPart(part meeting.Part, extracts bool) {
	p := a.printf
	head := part.Title
	if part.Number > 0 {
		head = fmt.Sprintf("%d. %s", part.Number, part.Title)
	}
	if part.Minutes > 0 {
		head += fmt.Sprintf(" (%d min.)", part.Minutes)
	}
	p("  %s\n", head)
	for _, t := range part.Text {
		p("     %s\n", t)
	}
	for _, q := range part.Questions {
		p("     ? %s\n", q)
	}
	for _, r := range part.References {
		if r.Kind == "bible" {
			continue
		}
		loc := firstNonEmpty(r.Location, r.Text)
		line := fmt.Sprintf("     → %s", loc)
		if r.Title != "" {
			line += " «" + r.Title + "»"
		}
		line += fmt.Sprintf(" · docid %d", r.DocID)
		if r.Pars != "" {
			line += " ¶" + r.Pars
		}
		if !r.InLibrary && r.Sync != "" {
			line += " · " + r.Sync
		}
		p("%s\n", line)
		if extracts && r.Extract != "" && part.Study == nil {
			for _, l := range strings.Split(r.Extract, "\n") {
				p("         %s\n", l)
			}
		}
	}
	var bibleRefs []bible.Range
	for _, r := range part.References {
		if r.Kind == "bible" && r.Range != nil {
			bibleRefs = append(bibleRefs, *r.Range)
		}
	}
	if len(bibleRefs) > 0 {
		p("     Scriptures: %s\n", bible.FormatList(bibleRefs))
	}
	for _, v := range part.Videos {
		p("     Video: %s %s", v.Key, v.Title)
		if v.Duration != "" {
			p(" (%s)", v.Duration)
		}
		p("\n")
	}
	for _, im := range part.Images {
		p("     Image: %s · %s\n", im.File, firstNonEmpty(im.Caption, im.Alt))
	}
	if sc := part.Study; sc != nil {
		p("     Chapter: %s · «%s» · %s · docid %d\n", sc.Label, sc.Title, sc.Location, sc.DocID)
		if len(sc.Accounts) > 0 {
			p("     Bible account: %s\n", strings.Join(sc.Accounts, "; "))
		}
		for _, g := range sc.Groups {
			p("     %s\n", g.Title)
			for _, q := range g.Questions {
				p("       ? %s\n", q)
			}
		}
		for _, r := range sc.References {
			p("       → %s · docid %d ¶%s\n", r.Text, r.DocID, r.Pars)
		}
		for _, v := range sc.Videos {
			p("     Video: %s %s", v.Key, v.Title)
			if v.Duration != "" {
				p(" (%s)", v.Duration)
			}
			p("\n")
		}
		if len(sc.Images) > 0 {
			p("     Chapter images: %d (pubkit image %d)\n", len(sc.Images), sc.DocID)
		}
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
