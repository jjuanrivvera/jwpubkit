package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"mime"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/jwpub"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type imageProvenance struct {
	DocID       int    `json:"docid"`
	Publication string `json:"publication"`
	BeginPID    int    `json:"begin_pid"`
	EndPID      int    `json:"end_pid"`
	Table       string `json:"table"`
}

func imageSelection(st *store.Store, docid int, parsed *content.Doc, paragraph, passage, figure, genre string) ([]*content.Image, map[string]imageProvenance, error) {
	if paragraph != "" && passage != "" {
		return nil, nil, fmt.Errorf("choose --paragraph or --passage")
	}
	doc, err := st.Doc(docid)
	if err != nil {
		return nil, nil, err
	}
	media, err := st.MediaOf(docid)
	if err != nil {
		return nil, nil, err
	}
	images := append([]*content.Image(nil), parsed.Images...)
	provenance := map[string]imageProvenance{}
	for _, img := range images {
		provenance[img.File] = imageProvenance{DocID: docid, Publication: doc.Pub.Key, BeginPID: img.PID, EndPID: img.PID, Table: "document HTML"}
	}
	for _, m := range media {
		if !strings.HasPrefix(m.Mime, "image/") || m.File == "" {
			continue
		}
		exists := false
		for _, img := range images {
			if img.File == m.File {
				exists = true
				break
			}
		}
		if !exists {
			images = append(images, &content.Image{File: m.File, Caption: content.InnerText(m.Caption), Alt: content.InnerText(m.Label), PID: m.BeginPID, Width: m.Width, Height: m.Height})
		}
		end := m.EndPID
		if end < m.BeginPID {
			end = m.BeginPID
		}
		provenance[m.File] = imageProvenance{DocID: docid, Publication: doc.Pub.Key, BeginPID: m.BeginPID, EndPID: end, Table: "media (DocumentMultimedia)"}
	}
	pids := map[int]bool{}
	restricted := paragraph != "" || passage != ""
	if paragraph != "" {
		bounds := strings.Split(paragraph, "-")
		first, err := strconv.Atoi(bounds[0])
		if err != nil || first < 1 || len(bounds) > 2 {
			return nil, nil, fmt.Errorf("paragraph must be N or N-M")
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(bounds[1])
			if err != nil || last < first {
				return nil, nil, fmt.Errorf("invalid paragraph range")
			}
		}
		for _, b := range parsed.Blocks {
			if b.Num >= first && b.Num <= last {
				pids[b.PID] = true
			}
		}
	}
	if passage != "" {
		ranges, err := bible.Parse(passage)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range ranges {
			citations, _, err := st.CitedBy(r.FirstID(), r.LastID(), 10000)
			if err != nil {
				return nil, nil, err
			}
			for _, c := range citations {
				if c.DocID == docid {
					for _, pid := range c.PIDs {
						pids[pid] = true
					}
				}
			}
		}
	}
	if genre != "" && genre != "svg" && genre != "photo" {
		return nil, nil, fmt.Errorf("genre must be svg or photo")
	}
	out := []*content.Image{}
	for _, img := range images {
		if figure != "" && img.File != figure {
			continue
		}
		svg := strings.EqualFold(path.Ext(img.File), ".svg")
		if (genre == "svg" && !svg) || (genre == "photo" && svg) {
			continue
		}
		p := provenance[img.File]
		selected := !restricted
		for pid := range pids {
			if pid >= p.BeginPID && pid <= p.EndPID {
				selected = true
				break
			}
		}
		if selected {
			out = append(out, img)
		}
	}
	return out, provenance, nil
}
func svgDims(data []byte) (int, int) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return 0, 0
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "svg" {
			return 0, 0
		}
		width, height := 0.0, 0.0
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "width":
				width, _ = strconv.ParseFloat(strings.TrimSuffix(a.Value, "px"), 64)
			case "height":
				height, _ = strconv.ParseFloat(strings.TrimSuffix(a.Value, "px"), 64)
			case "viewBox":
				fields := strings.Fields(a.Value)
				if len(fields) == 4 {
					width, _ = strconv.ParseFloat(fields[2], 64)
					height, _ = strconv.ParseFloat(fields[3], 64)
				}
			}
		}
		return int(width), int(height)
	}
}

var svgHref = regexp.MustCompile(`(?:xlink:)?href\s*=\s*["']([^"']+)["']`)

// Inline local SVG dependencies so exported figures survive outside their archive.
func inlineSVG(data []byte, file string, archive *jwpub.File) ([]byte, []string, error) {
	dependencies := []string{}
	var failure error
	result := svgHref.ReplaceAllStringFunc(string(data), func(attr string) string {
		match := svgHref.FindStringSubmatch(attr)
		ref := match[1]
		if strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "data:") {
			return attr
		}
		if strings.Contains(ref, ":") || strings.HasPrefix(ref, "/") {
			failure = fmt.Errorf("SVG has a nonlocal dependency %q", ref)
			return attr
		}
		name := path.Clean(path.Join(path.Dir(file), ref))
		b, err := archive.ReadFile(name)
		if err != nil {
			failure = fmt.Errorf("SVG dependency %s: %w", name, err)
			return attr
		}
		kind := mime.TypeByExtension(path.Ext(name))
		if kind == "" {
			kind = "application/octet-stream"
		}
		dependencies = append(dependencies, name)
		return strings.Replace(attr, ref, "data:"+kind+";base64,"+base64.StdEncoding.EncodeToString(b), 1)
	})
	if failure != nil {
		return nil, dependencies, failure
	}
	return []byte(result), dependencies, nil
}
