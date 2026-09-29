package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/jwpub"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type imgCandidate struct {
	Source string `json:"source"`
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
	data   []byte
}

type imgOut struct {
	File       string         `json:"file"`
	Alt        string         `json:"alt,omitempty"`
	Caption    string         `json:"caption,omitempty"`
	Credit     string         `json:"credit,omitempty"`
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	Bytes      int            `json:"bytes"`
	Source     string         `json:"source"`
	URL        string         `json:"url,omitempty"`
	SHA256     string         `json:"sha256"`
	Saved      string         `json:"saved,omitempty"`
	Media      string         `json:"media,omitempty"`
	Candidates []imgCandidate `json:"candidates"`
}

func (a *app) imageCmd() *cobra.Command {
	var outDir, mediaDir string
	var mediaStore, listOnly bool
	cmd := &cobra.Command{
		Use:     "image <docid>",
		Aliases: []string{"images", "imagen", "imagenes"},
		Short:   "A document's images at their largest resolution, with their captions",
		Long: `Collects a document's images and, for each one, compares the copy inside the JWPUB
with the versions on the jw.org image CDN (cms-imgp.jw-cdn.org, sizes xl and lg): it
keeps the one with the most pixels and, on a tie, the one with more bytes.

--media-store also writes each image to a content-addressed store,
<media-dir>/<xx>/<sha256>.<ext>, and prints the /media/... path for markdown.`,
		Example: `  pubkit image 202026255 --output /tmp/week
  pubkit image 1102025910 --media-store --list=false
  pubkit image 2026485 --list`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			docid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid docid %q", args[0])
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			out, parsed, err := a.loadDoc(st, docid)
			if err != nil {
				return err
			}
			if mediaStore && mediaDir == "" {
				mediaDir = a.defaultMediaDir()
			}
			local := a.localPubsFor(st, docid)
			var results []imgOut
			for _, img := range parsed.Images {
				if img.File == "" {
					continue
				}
				res := imgOut{File: img.File, Alt: img.Alt, Caption: img.Caption, Credit: img.Credit}
				cands := a.imageCandidates(img.File, local)
				if len(cands) == 0 {
					res.Source = "unavailable"
					results = append(results, res)
					continue
				}
				best := cands[0]
				for _, c := range cands[1:] {
					if c.Width*c.Height > best.Width*best.Height || (c.Width*c.Height == best.Width*best.Height && c.Bytes > best.Bytes) {
						best = c
					}
				}
				res.Width, res.Height, res.Bytes, res.Source, res.URL = best.Width, best.Height, best.Bytes, best.Source, best.URL
				res.Candidates = cands
				if best.data != nil {
					sum := sha256.Sum256(best.data)
					res.SHA256 = hex.EncodeToString(sum[:])
					if !listOnly {
						if outDir != "" || !mediaStore {
							dir := outDir
							if dir == "" {
								dir = "."
							}
							if err := os.MkdirAll(dir, 0o755); err != nil {
								return err
							}
							dest := filepath.Join(dir, filepath.Base(img.File))
							if err := os.WriteFile(dest, best.data, 0o644); err != nil {
								return err
							}
							res.Saved = dest
						}
						if mediaStore {
							m, err := storeMedia(mediaDir, res.SHA256, path.Ext(img.File), best.data)
							if err != nil {
								return err
							}
							res.Media = m
						}
					}
				}
				results = append(results, res)
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"docid": docid, "title": out.Title, "images": results, "media_dir": mediaDir})
			}
			a.printf("docid %d · %s · %d images\n", docid, out.Title, len(results))
			for i, r := range results {
				a.printf("%2d. %s  %d×%d  %s  (%s)", i+1, r.File, r.Width, r.Height, kb(r.Bytes), r.Source)
				var others []string
				for _, c := range r.Candidates {
					if c.Source != r.Source {
						others = append(others, fmt.Sprintf("%s %d×%d %s", c.Source, c.Width, c.Height, kb(c.Bytes)))
					}
				}
				if len(others) > 0 {
					a.printf(" · discarded: %s", strings.Join(others, ", "))
				}
				a.printf("\n")
				if r.Saved != "" {
					a.printf("    → %s\n", r.Saved)
				}
				if r.Media != "" {
					a.printf("    → %s\n", r.Media)
				}
				if r.Caption != "" {
					a.printf("    Caption: %s\n", r.Caption)
				}
				if r.Alt != "" {
					a.printf("    Alt: %s\n", r.Alt)
				}
				if r.Credit != "" {
					a.printf("    Credit: %s\n", r.Credit)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&outDir, "output", "o", "", "directory to write the images to (default: the current one)")
	cmd.Flags().BoolVar(&mediaStore, "media-store", false, "also write to <media-dir>/<xx>/<sha256>.<ext>")
	cmd.Flags().StringVar(&mediaDir, "media-dir", "", "content-addressed media store (default: <library>/media, or JWPUBKIT_MEDIA_DIR)")
	cmd.Flags().BoolVar(&listOnly, "list", false, "list only: do not reach the CDN and write nothing")
	return cmd
}

// localPubsFor lists the cached JWPUB files that may hold the images of a
// document: its own publication, and the one of the documents whose id
// prefixes the image names.
func (a *app) localPubsFor(st *store.Store, docid int) []string {
	var files []string
	if d, err := st.Doc(docid); err == nil && d.Pub.File != "" {
		files = append(files, d.Pub.File)
	}
	return files
}

func (a *app) imageCandidates(file string, localPubs []string) []imgCandidate {
	var cands []imgCandidate
	pubs := append([]string(nil), localPubs...)
	if st, err := a.store(); err == nil {
		if id, err := strconv.Atoi(leadingDigits(file)); err == nil {
			if d, err := st.Doc(id); err == nil && d.Pub.File != "" && !contains(pubs, d.Pub.File) {
				pubs = append(pubs, d.Pub.File)
			}
		}
	}
	for _, pf := range pubs {
		jf, err := jwpub.OpenContents(pf)
		if err != nil {
			continue
		}
		if data, err := jf.ReadFile(file); err == nil {
			c := imgCandidate{Source: "jwpub", Bytes: len(data), data: data}
			c.Width, c.Height = dims(data)
			cands = append(cands, c)
		}
		jf.Close()
		if len(cands) > 0 {
			break
		}
	}
	if a.offline {
		return cands
	}
	for _, u := range cdnImageURLs(file, a.lang) {
		data, err := a.client().GetBytes(a.ctx, u.url)
		if err != nil || len(data) == 0 {
			continue
		}
		c := imgCandidate{Source: "cdn " + u.size, URL: u.url, Bytes: len(data), data: data}
		c.Width, c.Height = dims(data)
		if c.Width == 0 {
			continue
		}
		cands = append(cands, c)
		break // "lg" is only a fallback: when "xl" exists it is never smaller
	}
	return cands
}

type cdnURL struct{ size, url string }

// cdnImageURLs builds cms-imgp addresses: /img/p/<docid>/<univ|S>/art/<name>_<size>.jpg.
func cdnImageURLs(file, lang string) []cdnURL {
	ext := path.Ext(file)
	base := strings.TrimSuffix(file, ext)
	prefix := leadingDigits(base)
	if prefix == "" || strings.Contains(base, "-") {
		return nil
	}
	seg := "univ"
	if strings.Contains(base, "_"+lang+"_") {
		seg = lang
	}
	var out []cdnURL
	for _, size := range []string{"xl", "lg"} {
		out = append(out, cdnURL{size, fmt.Sprintf("https://cms-imgp.jw-cdn.org/img/p/%s/%s/art/%s_%s%s", prefix, seg, base, size, ext)})
	}
	return out
}

func leadingDigits(s string) string {
	i := strings.IndexFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
	if i < 0 {
		return s
	}
	return s[:i]
}

func dims(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func kb(n int) string {
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}

// defaultMediaDir puts the media store beside the library unless the environment
// points somewhere else — a site that keeps its images in a repository sets the
// variable once instead of passing --media-dir on every call.
func (a *app) defaultMediaDir() string {
	for _, env := range []string{"JWPUBKIT_MEDIA_DIR", "JWLIB_MEDIA_DIR"} {
		if d := envOf(env); d != "" {
			return d
		}
	}
	if a.cfg.MediaDir != "" {
		return a.cfg.MediaDir
	}
	return filepath.Join(a.libDir, "media")
}

// envOf exists so the settings and the command that explains them read the
// environment through one door.
func envOf(name string) string { return os.Getenv(name) }

// storeMedia writes data as <dir>/<xx>/<sha256><ext> and returns the /media/...
// path, so the same image referenced from several documents is stored once.
func storeMedia(dir, sum, ext string, data []byte) (string, error) {
	ext = strings.ToLower(ext)
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	sub := filepath.Join(dir, sum[:2])
	dest := filepath.Join(sub, sum+ext)
	rel := "/media/" + sum[:2] + "/" + sum + ext
	if _, err := os.Stat(dest); err == nil {
		return rel, nil
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
