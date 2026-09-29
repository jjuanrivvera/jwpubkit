package content

import (
	"fmt"
	"strconv"
	"strings"
)

// Citation is a reference in the form publications use for themselves, plus the
// address that opens it.
type Citation struct {
	// Text is how the citation reads, e.g. "w24.05 p. 8 par. 3".
	Text string `json:"text"`
	// URL opens the cited paragraph.
	URL string `json:"url"`
}

// Cite builds a citation. Where a publication states its own — the caption of an
// extract is written by the publication, in its own language and conventions —
// that string is used verbatim, because nothing this tool composes can be more
// correct than what the publisher wrote. Otherwise it is assembled from the
// symbol, the issue, the page and the paragraph.
//
// The issue is rendered the way the publications themselves shortened it from
// 2016 on: 202405 becomes "w24.05". A pre-2016 dated issue (20130115) keeps its
// published form, "w13 15/1", because that is how it is cited everywhere.
func Cite(published, symbol, issue string, page, paragraph, docid, pid int) Citation {
	c := Citation{URL: WolDocURL(docid, pid)}
	if s := strings.TrimSpace(published); s != "" {
		c.Text = s
		return c
	}
	parts := []string{strings.TrimSpace(symbol + issueSuffix(symbol, issue))}
	if page > 0 {
		parts = append(parts, "p. "+strconv.Itoa(page))
	}
	if paragraph > 0 {
		parts = append(parts, "par. "+strconv.Itoa(paragraph))
	}
	c.Text = strings.TrimSpace(strings.Join(parts, " "))
	return c
}

// issueSuffix turns a stored issue number into the form used in citations. It
// returns "" for a publication with no issue, such as a book.
func issueSuffix(symbol, issue string) string {
	issue = strings.TrimSpace(issue)
	switch len(issue) {
	case 6: // YYYYMM, the monthly form: w24.05
		if strings.HasSuffix(symbol, issue[2:4]) {
			// The symbol already carries the year (mwb26), so only the month is new.
			return "." + issue[4:6]
		}
		return issue[2:4] + "." + issue[4:6]
	case 8: // YYYYMMDD, the pre-2016 dated form: w13 15/1
		day := strings.TrimPrefix(issue[6:8], "0")
		month := strings.TrimPrefix(issue[4:6], "0")
		return fmt.Sprintf("%s %s/%s", issue[2:4], day, month)
	}
	return ""
}
