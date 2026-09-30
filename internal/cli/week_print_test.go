package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
)

// printWeek is what the reader actually sees. The shape matters: the header
// says which week, each section names its parts with their number and minutes,
// and a Bible reference inside a part is not repeated as a pointer to another
// publication — it is already printed as the reference it is.
func TestPrintWeekRendersTheWholeMeeting(t *testing.T) {
	var out bytes.Buffer
	a := &app{out: &out}

	w := &meeting.Week{
		Monday: "2026-01-05",
		Range:  "5-11 January",
		Workbook: &meeting.DocRef{
			DocID: 2026101, Location: "mwb26.01 p. 3", URL: "https://www.jw.org/finder?docid=2026101",
		},
		WeeklyReading:  &meeting.Reading{Text: "An invented book 1-3", Ref: "Inv 1:1-3:24"},
		StudentReading: &meeting.Assignment{Ref: "Inv 1:1-18", Lesson: &meeting.Reference{Text: "lesson 4", Title: "An Invented Lesson", DocID: 2026400}},
		Songs: []meeting.Song{
			{Number: 12, Title: "An Invented Song", When: "opening"},
			{Number: 99, Title: "Another Invented Song", When: "closing"},
		},
		Sections: []meeting.Section{{
			Title: "TREASURES FROM AN INVENTED BOOK",
			Parts: []meeting.Part{{
				Number: 1, Title: "An invented talk", Minutes: 10,
				Text:      []string{"An invented line of the part."},
				Questions: []string{"An invented question?"},
				References: []meeting.Reference{
					{Kind: "bible", Text: "Inv 1:1"},
					{Kind: "pub", Location: "it-1 p. 44", Title: "An Invented Entry"},
				},
			}},
		}},
		Videos: []meeting.Video{{Key: "pub-inv-1_1_VIDEO", Title: "An Invented Video"}},
	}

	a.printWeek(w, false)
	got := out.String()

	for _, want := range []string{
		"Week of 5-11 January (monday 2026-01-05)",
		"docid 2026101",
		"Weekly Bible reading: An invented book 1-3 (Inv 1:1-3:24)",
		"Student reading: Inv 1:1-18",
		"An Invented Lesson",
		"12 «An Invented Song» (opening)",
		"99 «Another Invented Song» (closing)",
		"TREASURES FROM AN INVENTED BOOK",
		"1. An invented talk (10 min.)",
		"An invented line of the part.",
		"? An invented question?",
		"→ it-1 p. 44 «An Invented Entry»",
		"pub-inv-1_1_VIDEO",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the printed week is missing %q:\n%s", want, got)
		}
	}

	// A Bible reference is already printed as the reference it is; repeating it
	// as a pointer to another publication would double every scripture.
	if strings.Contains(got, "→ Inv 1:1") {
		t.Errorf("a bible reference should not be repeated as a publication pointer:\n%s", got)
	}
}

// The header has to survive a week with nothing in it: the days are known long
// before the workbook is.
func TestPrintWeekOfAnEmptyWeek(t *testing.T) {
	var out bytes.Buffer
	a := &app{out: &out}
	a.printWeek(&meeting.Week{Monday: "2026-01-05", Range: "5-11 January"}, false)

	got := out.String()
	if !strings.Contains(got, "2026-01-05") {
		t.Errorf("an empty week should still say which week it is: %q", got)
	}
	if strings.Contains(got, "Songs:") || strings.Contains(got, "Meeting videos") {
		t.Errorf("an empty week should not announce sections it does not have: %q", got)
	}
}

// A part with no number and no duration must not print "0." or "(0 min.)".
func TestPrintPartOmitsWhatItDoesNotKnow(t *testing.T) {
	var out bytes.Buffer
	a := &app{out: &out}
	a.printPart(meeting.Part{Title: "An untitled-position part"}, false)

	got := out.String()
	if strings.Contains(got, "0.") || strings.Contains(got, "0 min") {
		t.Errorf("a part with nothing to say about its number or length should say nothing: %q", got)
	}
	if !strings.Contains(got, "An untitled-position part") {
		t.Errorf("the part should still be printed: %q", got)
	}
}
