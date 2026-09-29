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
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

type subsOut struct {
	Key        string     `json:"clave"`
	Title      string     `json:"titulo"`
	Duration   string     `json:"duracion"`
	Source     string     `json:"fuente"`
	VTT        string     `json:"vtt"`
	Transcript string     `json:"transcripcion"`
	Cues       []subs.Cue `json:"subtitulos,omitempty"`
}

var pubKeyRe = regexp.MustCompile(`^pub-(.+?)(?:_(\d{6,8}))?_(\d+)_VIDEO$`)

func (a *app) subtitulosCmd() *cobra.Command {
	var format string
	var withTimes bool
	cmd := &cobra.Command{
		Use:     "subtitulos <clave-del-video>",
		Aliases: []string{"subtítulos", "subs", "transcripcion"},
		Short:   "Transcripción de un video de la JW desde sus subtítulos",
		Long: `Pide el video a la API mediator (b.jw-cdn.org/apis/mediator/v1/media-items/S/<clave>),
toma el archivo de subtítulos (WebVTT) y lo convierte en una transcripción legible. Si el
mediator no trae subtítulos, prueba el campo "subtitles" de pub-media. "pubkit semana"
lista las claves de los videos de la reunión.

La clave puede ser pub-jwb-125_4_VIDEO, docid-702017141_1_VIDEO, un enlace de jw.org
(finder?lank=...), webpubvid://?pub=...&track=... o la forma corta pub:track
(jwb-125:4) o pub:issue:track (jwbai:201507:1).`,
		Example: `  pubkit subtitulos pub-jwb-125_4_VIDEO
  pubkit subtitulos jwb-125:4 --tiempos
  pubkit subtitulos pub-jwbcov21_11_VIDEO --formato vtt > video.vtt`,
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
			vttPath := filepath.Join(st.Dir, "subtitulos", key+"."+a.lang+".vtt")
			out := subsOut{Key: key}
			var vtt []byte
			if cached, _ := st.Video(key, a.lang); cached != nil {
				out.Title = cached.Title
				out.Duration = subs.FormatTS(time.Duration(cached.Duration * float64(time.Second)))
				if b, err := os.ReadFile(vttPath); err == nil {
					vtt, out.Source, out.VTT = b, "caché", cached.Subtitles
				}
			}
			if vtt == nil {
				if a.offline {
					return fmt.Errorf("los subtítulos de %s no están en caché: %w", key, errOffline)
				}
				u, title, dur, src, err := a.findSubtitles(key)
				if err != nil {
					return err
				}
				out.Title, out.Duration, out.Source, out.VTT = title, dur, src, u
				if vtt, err = a.client().GetBytes(a.ctx, u); err != nil {
					return fmt.Errorf("descargando %s: %w", u, err)
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
			if a.jsonOut || format == "json" {
				out.Cues = cues
				return a.printJSON(out)
			}
			a.printf("%s\n%s · %s · subtítulos: %s\n\n", out.Title, key, out.Duration, out.VTT)
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
	cmd.Flags().StringVarP(&format, "formato", "f", "txt", "txt, json o vtt")
	cmd.Flags().BoolVar(&withTimes, "tiempos", false, "una línea por subtítulo con su marca de tiempo")
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
			return "", "", "", "", fmt.Errorf("%s: ni el mediator ni pub-media traen subtítulos (%d archivos MP4 en pub-media, ninguno con subtitles.url)", key, len(pm.Files[a.lang]["MP4"]))
		}
		if merr != nil {
			return "", "", "", "", fmt.Errorf("%s: mediator: %v; pub-media: %v", key, merr, perr)
		}
	}
	if merr != nil {
		if errors.Is(merr, cdn.ErrNotFound) {
			return "", "", "", "", fmt.Errorf("el mediator no conoce %s en el idioma %s (404)", key, a.lang)
		}
		return "", "", "", "", merr
	}
	return "", "", "", "", fmt.Errorf("el mediator devolvió «%s» (%s) sin subtítulos en ninguna de sus %d versiones", title, key, len(item.Files))
}
