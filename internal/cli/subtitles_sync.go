package cli

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

func (a *app) subtitlesSyncCmd() *cobra.Command {
	var catalog, noVideo, resume, fallback bool
	var budget int64
	var interval time.Duration
	cmd := &cobra.Command{Use: "sync", Short: "Index official catalog subtitles without downloading videos", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if !catalog || !noVideo {
			return fmt.Errorf("use --catalog --no-video")
		}
		if budget <= 0 || interval < 0 {
			return fmt.Errorf("budget must be positive and interval nonnegative")
		}
		cat, err := a.readCatalog()
		if err != nil {
			return err
		}
		st, err := a.commandStore(cmd)
		if err != nil {
			return err
		}
		if err := st.PutCatalog(cat.Media, a.lang); err != nil {
			return err
		}
		transferred := int64(0)
		indexed, skipped, failed := 0, 0, 0
		for _, item := range cat.Media {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			key := item.LanguageAgnosticNaturalKey
			u := item.SubtitlesURL()
			checksum, modified := "", ""
			for _, f := range item.Files {
				if f.Subtitles != nil && f.Subtitles.URL == u {
					checksum = f.Subtitles.Checksum
					modified = f.Subtitles.ModifiedDatetime
					break
				}
			}
			if u == "" {
				if !fallback {
					continue
				}
				if a.offline {
					return errOffline
				}
				if err := cdn.Pause(cmd.Context(), interval); err != nil {
					return err
				}
				var err error
				u, checksum, err = a.pubMediaSubtitles(key)
				if err != nil {
					failed++
					if e := st.CatalogState(key, a.lang, "lookup_failed", checksum, err.Error()); e != nil {
						return e
					}
					continue
				}
				if u == "" {
					continue
				}
				if err := st.PutVideo(key, a.lang, item.Title, item.Duration, u, item); err != nil {
					return err
				}
			}
			revision := checksum
			if revision == "" {
				revision = fmt.Sprintf("url:%x", sha256.Sum256([]byte(u+"\n"+modified)))
			}
			path := filepath.Join(st.Dir, subtitleDir, key+"."+a.lang+".vtt")
			b, cacheErr := readCachedVTT(st.Dir, path)
			sum := md5.Sum(b)
			cached := cacheErr == nil && (checksum == "" || strings.EqualFold(hex.EncodeToString(sum[:]), checksum))
			var indexedChecksum string
			if err := st.DB.QueryRowContext(cmd.Context(), `SELECT checksum FROM media_catalog WHERE key=? AND lang=?`, key, a.lang).Scan(&indexedChecksum); err != nil {
				return err
			}
			if checksum == "" && indexedChecksum != revision {
				cached = false
			}
			if resume && cached && indexedChecksum == revision && st.HasCues(key, a.lang) {
				skipped++
				if err := st.CatalogState(key, a.lang, "indexed", revision, ""); err != nil {
					return err
				}
				continue
			}
			if !cached {
				if a.offline {
					return errOffline
				}
				if transferred >= budget {
					return fmt.Errorf("subtitle byte budget exhausted; rerun with --resume")
				}
				if err := cdn.Pause(cmd.Context(), interval); err != nil {
					return err
				}
				var n int64
				b, n, err = a.client().GetLimited(cmd.Context(), u, budget-transferred, checksum)
				transferred += n
				if err != nil {
					failed++
					if e := st.CatalogState(key, a.lang, "failed", checksum, err.Error()); e != nil {
						return e
					}
					a.logf("%s: %v", key, err)
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(path+".part", b, 0o644); err != nil {
					return err
				}
				if err := os.Rename(path+".part", path); err != nil {
					return err
				}
			}
			cues, err := subs.ParseVTT(string(b))
			if err != nil {
				failed++
				if e := st.CatalogState(key, a.lang, "failed", checksum, err.Error()); e != nil {
					return e
				}
				continue
			}
			starts, ends := make([]time.Duration, len(cues)), make([]time.Duration, len(cues))
			texts := make([]string, len(cues))
			for i, c := range cues {
				starts[i], ends[i], texts[i] = c.Start, c.End, subsLine(c.Text)
			}
			if err := st.PutCues(key, a.lang, starts, ends, texts); err != nil {
				return err
			}
			if err := st.CatalogState(key, a.lang, "indexed", revision, ""); err != nil {
				return err
			}
			indexed++
			a.logf("subtitles: indexed %d, cached %d, failed %d, received %d bytes (%s)", indexed, skipped, failed, transferred, key)
		}
		coverage, err := st.CatalogCoverage(a.lang)
		if err != nil {
			return err
		}
		if a.jsonOut {
			if err := a.printJSON(map[string]any{"indexed": indexed, "skipped": skipped, "failed": failed, "received_bytes": transferred, "coverage": coverage}); err != nil {
				return err
			}
		} else {
			a.printf("Indexed %d; resumed %d; failed %d; received %d bytes\nCoverage: %v\n", indexed, skipped, failed, transferred, coverage)
		}
		if failed > 0 {
			return fmt.Errorf("%d subtitle downloads failed; rerun with --resume", failed)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&fallback, "pub-media-fallback", false, "also probe pub-media for catalog items without a mediator VTT (additional requests)")
	cmd.Flags().BoolVar(&catalog, "catalog", false, "use the local complete media catalog")
	cmd.Flags().BoolVar(&noVideo, "no-video", false, "download subtitle files only")
	cmd.Flags().BoolVar(&resume, "resume", true, "reuse checksum-matching indexed transcripts")
	cmd.Flags().Int64Var(&budget, "budget-bytes", 100000000, "maximum subtitle response bytes received this run")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "pause before each subtitle request")
	return cmd
}
