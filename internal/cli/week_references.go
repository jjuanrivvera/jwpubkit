package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func referencePublication(e store.Extract) (string, string) {
	symbol := e.RefUndated
	if symbol == "" {
		symbol = e.RefSymbol
		if e.RefIssue != 0 {
			symbol = strings.TrimRight(symbol, "0123456789")
		}
	}
	issue := ""
	if e.RefIssue != 0 {
		issue = store.NormalizeIssue(strconv.Itoa(e.RefIssue))
	}
	return symbol, issue
}
func (a *app) updateReferences(st *store.Store, monday time.Time, dryRun bool, interval time.Duration, out *weekUpdateOut) error {
	if st == nil {
		return nil
	}
	builder := &meeting.Builder{Store: st}
	dd, err := builder.FindWorkbook(monday)
	if err != nil {
		return err
	}
	if dd == nil {
		return nil
	}
	extracts, err := st.ExtractsOf(dd.DocID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	failures := 0
	for _, e := range extracts {
		if _, err := st.Doc(e.RefDocID); err == nil {
			continue
		}
		symbol, issue := referencePublication(e)
		if symbol == "" {
			out.Notes = append(out.Notes, fmt.Sprintf("Reference docid %d lacks publication metadata; resolve manually.", e.RefDocID))
			continue
		}
		key := store.PubKey(symbol, a.lang, issue)
		if seen[key] {
			continue
		}
		seen[key] = true
		row := weekUpdateRow{Kind: "publication_reference", Key: key, Symbol: symbol, Issue: issue, Status: "would sync missing reference", DocID: e.RefDocID}
		if !dryRun {
			if err := cdn.Pause(a.ctx, interval); err != nil {
				return err
			}
			result, err := a.syncOne(symbol, issue, false)
			if err != nil {
				row.Status, row.Error = "failed", err.Error()
				failures++
			} else if result.UpToDate {
				row.Status = "already current"
			} else {
				row.Status = "synced"
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if failures > 0 {
		return fmt.Errorf("%d referenced publications failed", failures)
	}
	return nil
}
