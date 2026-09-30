package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

type subsOut struct {
	Key        string     `json:"key"`
	Title      string     `json:"title"`
	Duration   string     `json:"duration"`
	Source     string     `json:"source"`
	VTT        string     `json:"vtt"`
	Transcript string     `json:"transcript"`
	Cues       []subs.Cue `json:"subtitles,omitempty"`
}

var pubKeyRe = regexp.MustCompile(`^pub-(.+?)(?:_(\d{6,8}))?_(\d+)_VIDEO$`)

const (
	subtitleDir = "subtitles"
	// Libraries built before the CLI spoke English cached VTT files here.
	legacySubtitleDir = "subtitulos"
)

// readCachedVTT prefers the current directory and falls back to the old one, so
// upgrading does not silently re-download every transcript already on disk.
func readCachedVTT(libDir, path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		return b, nil
	}
	return os.ReadFile(filepath.Join(libDir, legacySubtitleDir, filepath.Base(path)))
}

func (a *app) subtitlesCmd() *cobra.Command {
	var format string
	var withTimes bool
	cmd := &cobra.Command{
		Use:     "subtitles <video-key>",
		Aliases: []string{"subs", "subtitulos", "subtítulos"},
		Short:   "Transcribe a video from its subtitles",
		Long: `Asks the mediator API for the video, takes its subtitle file (WebVTT) and turns it
into a readable transcript. When the mediator carries no subtitles it falls back to
pub-media's "subtitles" field. "pubkit week" lists the keys of the meeting's videos.

The key can be pub-jwb-125_4_VIDEO, docid-702017141_1_VIDEO, a jw.org link
(finder?lank=...), webpubvid://?pub=...&track=... or the short forms pub:track
(jwb-125:4) and pub:issue:track (jwbai:201507:1).`,
		Example: `  pubkit subtitles pub-jwb-125_4_VIDEO
  pubkit subtitles jwb-125:4 --timings
  pubkit subtitles pub-jwbcov21_11_VIDEO --format vtt > video.vtt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := subs.NormalizeKey(args[0])
			if err != nil {
				return err
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			vttPath := filepath.Join(st.Dir, subtitleDir, key+"."+a.lang+".vtt")
			out := subsOut{Key: key}
			var vtt []byte
			if cached, _ := st.Video(key, a.lang); cached != nil {
				out.Title = cached.Title
				out.Duration = subs.FormatTS(time.Duration(cached.Duration * float64(time.Second)))
				if b, err := readCachedVTT(st.Dir, vttPath); err == nil {
					vtt, out.Source, out.VTT = b, "cache", cached.Subtitles
				}
			}
			if vtt == nil {
				if a.offline {
					return fmt.Errorf("the subtitles for %s are not cached: %w", key, errOffline)
				}
				u, title, dur, src, err := a.findSubtitles(key)
				if err != nil {
					return err
				}
				out.Title, out.Duration, out.Source, out.VTT = title, dur, src, u
				if vtt, err = a.client().GetBytes(a.ctx, u); err != nil {
					return fmt.Errorf("downloading %s: %w", u, err)
				}
				if err := os.MkdirAll(filepath.Dir(vttPath), 0o755); err == nil {
					_ = os.WriteFile(vttPath, vtt, 0o644)
				}
			}
			if format == "vtt" {
				_, err := a.out.Write(vtt)
				return err
			}
			cues, err := subs.ParseVTT(string(vtt))
			if err != nil {
				return err
			}
			out.Transcript = subs.Transcript(cues)
			// Recording the cues is what makes them findable later: `subtitles
			// find` searches what has been fetched, so fetching one transcript
			// adds a video to the searchable set.
			a.recordCues(st, key, cues)
			if a.jsonOut || format == "json" {
				out.Cues = cues
				return a.printJSON(out)
			}
			a.printf("%s\n%s · %s · subtitles: %s\n\n", out.Title, key, out.Duration, out.VTT)
			if withTimes {
				for _, c := range cues {
					a.printf("[%s] %s\n", c.From, subsLine(c.Text))
				}
				return nil
			}
			a.printf("%s\n", out.Transcript)
			return nil
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "txt", "txt, json or vtt")
	cmd.Flags().BoolVar(&withTimes, "timings", false, "one line per cue, with its timestamp")
	cmd.AddCommand(a.findCmd())
	return cmd
}

// recordCues stores a transcript for searching. A failure here must not spoil the
// command the user actually asked for, so it is noted on stderr and dropped.
func (a *app) recordCues(st *store.Store, key string, cues []subs.Cue) {
	starts := make([]time.Duration, len(cues))
	ends := make([]time.Duration, len(cues))
	texts := make([]string, len(cues))
	for i, c := range cues {
		starts[i], ends[i], texts[i] = c.Start, c.End, subsLine(c.Text)
	}
	if err := st.PutCues(key, a.lang, starts, ends, texts); err != nil {
		fmt.Fprintf(a.err, "note: the transcript of %s could not be indexed for searching: %v\n", key, err)
	}
}

// findCmd searches the transcripts already fetched and says where in the video
// each hit is. The second matters more than the video: it is what lets someone
// open the clip at the line they were looking for instead of watching for it.
func (a *app) findCmd() *cobra.Command {
	var limit int
	var allLangs bool
	cmd := &cobra.Command{
		Use:     `find "<query>"`,
		Aliases: []string{"search", "buscar"},
		Short:   "Find a phrase in the transcripts already fetched, with its timestamp",
		Long: `Searches the subtitles of every video whose transcript this library has fetched,
and reports the video, the exact second the phrase is said, and a link that opens
it there.

Only fetched transcripts are searched — "pubkit subtitles <key>" is what adds a
video to the set, and "pubkit week" lists the keys of a week's videos. Nothing is
downloaded by this command.`,
		Example: `  pubkit subtitles find "cistern"
  pubkit subtitles find "\"exact phrase\"" --limit 5`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			lang := a.lang
			if allLangs {
				lang = ""
			}
			hits, err := st.SearchCues(args[0], lang, limit)
			if err != nil {
				return err
			}
			type found struct {
				store.CueHit
				Timestamp string `json:"timestamp"`
				URL       string `json:"url"`
				// UsedIn are the documents that embed this video, which is what
				// turns "the phrase is at 1:04" into "and it belongs to this part
				// of this article".
				UsedIn []store.Edge `json:"used_in,omitempty"`
			}
			out := make([]found, 0, len(hits))
			for _, h := range hits {
				f := found{CueHit: h, Timestamp: subs.FormatTS(h.Start), URL: subs.WatchURL(h.Key, h.Lang, h.Seconds)}
				if docs, err := st.VideoDocuments(h.Key, 3); err == nil {
					f.UsedIn = docs
				}
				out = append(out, f)
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"query": args[0], "hits": out})
			}
			if len(out) == 0 {
				n, _ := st.CueVideoCount(lang)
				if n == 0 {
					a.printf("No transcripts have been fetched yet. Fetch one with: pubkit subtitles <key>\n")
					return nil
				}
				a.printf("Nothing found for %q in the transcripts of %d videos.\n", args[0], n)
				return nil
			}
			for _, h := range out {
				a.printf("%-28s %7s  %s\n", h.Key, h.Timestamp, h.Title)
				a.printf("%38s%s\n", "", h.Snippet)
				a.printf("%38s%s\n", "", h.URL)
				for _, d := range h.UsedIn {
					a.printf("%38sused in %s  %s\n", "", d.Cite, d.Title)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of hits")
	cmd.Flags().BoolVar(&allLangs, "all-languages", false, "search transcripts of every language, not just --language")
	return cmd
}

func subsLine(s string) string {
	return regexp.MustCompile(`\s*\n\s*`).ReplaceAllString(s, " ")
}

// findSubtitles asks the mediator first and pub-media second.
func (a *app) findSubtitles(key string) (vttURL, title, dur, source string, err error) {
	st, _ := a.store()
	item, merr := a.client().MediaItem(a.ctx, key)
	if merr == nil {
		title = item.Title
		dur = subs.FormatTS(time.Duration(item.Duration * float64(time.Second)))
		_ = st.PutVideo(key, a.lang, item.Title, item.Duration, item.SubtitlesURL(), item)
		if u := item.SubtitlesURL(); u != "" {
			return u, title, dur, "mediator", nil
		}
	}
	// pub-media knows pub/track videos too, and lists "subtitles" per file.
	if m := pubKeyRe.FindStringSubmatch(key); m != nil {
		track, _ := strconv.Atoi(m[3])
		pm, perr := a.client().PubMedia(a.ctx, cdn.PubMediaQuery{Pub: m[1], Issue: m[2], Track: track, Format: "MP4"})
		if perr == nil {
			for _, f := range pm.Files[a.lang]["MP4"] {
				if f.Subtitles != nil && f.Subtitles.URL != "" {
					if title == "" {
						title = f.Title
					}
					if dur == "" {
						dur = subs.FormatTS(time.Duration(f.Duration * float64(time.Second)))
					}
					return f.Subtitles.URL, title, dur, "pub-media", nil
				}
			}
			return "", "", "", "", fmt.Errorf("%s: neither the mediator nor pub-media carries subtitles (%d MP4 files in pub-media, none with subtitles.url)", key, len(pm.Files[a.lang]["MP4"]))
		}
		if merr != nil {
			return "", "", "", "", fmt.Errorf("%s: mediator: %v; pub-media: %v", key, merr, perr)
		}
	}
	if merr != nil {
		if errors.Is(merr, cdn.ErrNotFound) {
			return "", "", "", "", fmt.Errorf("the mediator does not know %s in language %s (404)", key, a.lang)
		}
		return "", "", "", "", merr
	}
	return "", "", "", "", fmt.Errorf("the mediator returned «%s» (%s) with no subtitles in any of its %d versions", title, key, len(item.Files))
}
