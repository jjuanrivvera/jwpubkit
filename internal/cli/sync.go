package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

func (a *app) syncCmd() *cobra.Command {
	var issue, file string
	var force, plan bool
	var interval time.Duration
	var budget int64
	cmd := &cobra.Command{
		Use:   "sync <symbol>...",
		Short: "Download (cached, checksum-verified), decrypt and index publications",
		Long: `Downloads each symbol's JWPUB from the jw.org pub-media API, checks its MD5,
decrypts it and indexes it into the library. Nothing is downloaded when the local
copy already matches the CDN checksum.

Symbols worth knowing: mwb (meeting workbook, with --issue YYYYMM), w (study
Watchtower, with --issue YYYYMM; before 2016 it is YYYYMMDD), nwtsty (study Bible),
it, wcg, lmd, th, jr, gl, lff, ijwia, sjj…

Publications are fetched in the language given by --language (default E).`,
		Example: `  pubkit sync mwb --issue 202609
  pubkit sync w --issue 202607
  pubkit sync nwtsty it wcg
  pubkit sync w --issue 20130115 --language S
  pubkit sync --file ~/Downloads/mwb_E_202609.jwpub mwb --issue 202609`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.ctx = cmd.Context()
			if interval < 0 || budget < 0 {
				return fmt.Errorf("interval and budget must not be negative")
			}
			if plan {
				if file != "" {
					return fmt.Errorf("--plan cannot be combined with --file")
				}
				rows, err := a.planSync(args, issue, force, interval)
				if err != nil {
					return err
				}
				return a.printSyncPlan(rows)
			}
			st, err := a.commandStore(cmd)
			if err != nil {
				return err
			}
			if file != "" {
				if len(args) != 1 {
					return fmt.Errorf("--file takes exactly one symbol")
				}
				stats, err := st.IndexLocal(file, args[0], issue, a.lang)
				if err != nil {
					return err
				}
				if a.jsonOut {
					return a.printJSON(map[string]any{"key": store.PubKey(args[0], a.lang, store.NormalizeIssue(issue)), "summary": stats.String(), "indexed_ms": stats.Elapsed.Milliseconds()})
				}
				a.printf("✓ %s indexed from %s in %s: %s\n", store.PubKey(args[0], a.lang, store.NormalizeIssue(issue)), file, ms(stats.Elapsed), stats)
				return nil
			}
			var results []*store.SyncResult
			var failed []string
			used := int64(0)
			pending := []string{}
			for i, sym := range args {
				if i > 0 {
					if err := cdn.Pause(cmd.Context(), interval); err != nil {
						return err
					}
				}
				if budget > 0 {
					rows, err := a.planSync([]string{sym}, issue, force, 0)
					if err != nil {
						return err
					}
					if rows[0].DownloadBytes > budget-used {
						pending = append(pending, sym)
						continue
					}
					if err := cdn.Pause(cmd.Context(), interval); err != nil {
						return err
					}
				}
				res, err := a.syncOne(strings.TrimSpace(sym), issue, force)
				if err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", sym, err))
					if !a.jsonOut {
						fmt.Fprintf(a.err, "✗ %s: %v\n", sym, err)
					}
					continue
				}
				if res.Downloaded {
					used += res.Size
				}
				results = append(results, res)
				if a.jsonOut {
					continue
				}
				switch {
				case res.UpToDate:
					a.printf("✓ %s up to date (%s, md5 %s)\n  %s\n", res.Key, mb(res.Size), res.MD5, res.Title)
				default:
					dl := "cached copy verified"
					if res.Downloaded {
						dl = fmt.Sprintf("downloaded %s in %s", mb(res.Size), ms(res.Download))
					}
					a.printf("✓ %s · %s · indexed in %s\n  %s\n  %s\n", res.Key, dl, ms(res.Stats.Elapsed), res.Title, res.Summary)
				}
			}
			if a.jsonOut {
				if err := a.printJSON(map[string]any{"synced": results, "errors": failed, "pending": pending, "download_bytes": used}); err != nil {
					return err
				}
			}
			if len(pending) > 0 {
				return fmt.Errorf("%d publications pending: byte budget exhausted (%s)", len(pending), strings.Join(pending, ", "))
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d of %d publications failed", len(failed), len(args))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&plan, "plan", false, "show checksums, download bytes and disk estimate without writes")
	cmd.Flags().Int64Var(&budget, "budget-bytes", 0, "maximum planned publication download bytes (0: unlimited)")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "pause between publication requests")
	cmd.Flags().StringVar(&issue, "issue", "", "issue of a periodical: YYYYMM (or YYYYMMDD before 2016)")
	cmd.Flags().BoolVar(&force, "force", false, "download and index even when the local copy is up to date")
	cmd.Flags().StringVar(&file, "file", "", "index a local .jwpub instead of downloading it")
	return cmd
}

func (a *app) pubsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "pubs",
		Aliases: []string{"library", "list", "biblioteca", "lista"},
		Short:   "List the publications held in the local library",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			pubs, err := st.Pubs()
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(pubs)
			}
			if len(pubs) == 0 {
				a.printf("The library %s is empty. Start with: pubkit sync mwb --issue YYYYMM\n", a.libDir)
				return nil
			}
			var total int64
			for _, p := range pubs {
				total += p.Size
				a.printf("%-18s %-8s %5d docs  %9s  %s\n", p.Key, p.MepsSymbol, p.Docs, mb(p.Size), p.Title)
			}
			a.printf("%d publications, %s cached in %s\n", len(pubs), mb(total), st.PubsDir())
			return nil
		},
	}
}
