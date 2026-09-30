package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type publicationListing struct {
	Symbol  string                   `json:"symbol"`
	Issue   string                   `json:"issue"`
	Title   string                   `json:"title"`
	Formats map[string][]cdn.PubFile `json:"formats"`
}

func (a *app) publicationCatalogCmd() *cobra.Command {
	var symbol, issue, format string
	var year int
	var interval time.Duration
	cmd := &cobra.Command{Use: "publications [symbol]", Short: "List available formats by symbol, year and issue", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if a.offline {
			return errOffline
		}
		a.ctx = cmd.Context()
		if len(args) > 0 {
			if symbol != "" {
				return fmt.Errorf("choose a positional symbol or --symbol")
			}
			symbol = args[0]
		}
		if symbol == "" {
			return fmt.Errorf("a publication symbol is required")
		}
		if year < 0 || year > 9999 || (year > 0 && year < 1000) {
			return fmt.Errorf("year must be YYYY")
		}
		if year > 0 && issue != "" {
			return fmt.Errorf("choose --year or --issue")
		}
		if interval < 0 {
			return fmt.Errorf("interval must not be negative")
		}
		issues := []string{store.NormalizeIssue(issue)}
		if year > 0 {
			issues = nil
			for month := 1; month <= 12; month++ {
				prefix := fmt.Sprintf("%04d%02d", year, month)
				if (symbol == "w" || symbol == "wp") && year < 2016 {
					issues = append(issues, prefix+"01", prefix+"15")
				} else {
					issues = append(issues, prefix)
				}
			}
		}
		out := []publicationListing{}
		for i, n := range issues {
			if i > 0 {
				if err := cdn.Pause(a.ctx, interval); err != nil {
					return err
				}
			}
			pm, err := a.client().PubMedia(a.ctx, cdn.PubMediaQuery{Pub: symbol, Issue: n, Format: strings.ToUpper(format)})
			if errors.Is(err, cdn.ErrNotFound) && year > 0 {
				continue
			}
			if err != nil {
				return err
			}
			files := pm.Files[a.lang]
			filtered := map[string][]cdn.PubFile{}
			for kind, f := range files {
				if format == "" || strings.EqualFold(kind, format) {
					if len(f) > 0 {
						filtered[kind] = f
					}
				}
			}
			if len(filtered) > 0 {
				out = append(out, publicationListing{Symbol: symbol, Issue: n, Title: pm.PubName, Formats: filtered})
			}
		}
		if a.jsonOut {
			return a.printJSON(out)
		}
		for _, p := range out {
			keys := []string{}
			for k := range p.Formats {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			a.printf("%s %s · %s · %s\n", p.Symbol, p.Issue, p.Title, strings.Join(keys, ", "))
		}
		return nil
	}}
	cmd.Flags().StringVar(&symbol, "symbol", "", "publication symbol")
	cmd.Flags().IntVar(&year, "year", 0, "probe issues in this year, including historical half-month issues")
	cmd.Flags().StringVar(&issue, "issue", "", "one issue: YYYYMM or YYYYMMDD")
	cmd.Flags().StringVar(&format, "format", "", "only this format, e.g. JWPUB, PDF, MP4 (default: all)")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "pause between issue requests")
	return cmd
}
