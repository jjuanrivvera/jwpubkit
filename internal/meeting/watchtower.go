package meeting

import (
	"strings"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/content"
)

// WatchtowerArticle adds the article's body to the meeting's study outline.
// Keeping it separate preserves the week command's existing output contract.
type WatchtowerArticle struct {
	Watchtower
	Paragraphs []StudyParagraph `json:"paragraphs"`
}

// StudyParagraph keeps the publisher's question pointer and scripture links.
type StudyParagraph struct {
	PID        int      `json:"pid"`
	Number     int      `json:"number"`
	Text       string   `json:"text"`
	QuestionID int      `json:"question_pid,omitempty"`
	Question   string   `json:"question"`
	Scriptures []string `json:"scriptures"`
	Images     []Image  `json:"images"`
}

// BuildWatchtowerArticle reuses the meeting parser, then follows data-rel-pid
// instead of guessing question ownership from adjacent paragraph numbers.
func (b *Builder) BuildWatchtowerArticle(docid int) (*WatchtowerArticle, error) {
	w := &Week{}
	if err := b.BuildWatchtower(w, docid); err != nil {
		return nil, err
	}
	d, err := b.Store.Doc(docid)
	if err != nil {
		return nil, err
	}
	parsed, err := content.Parse(d.HTML)
	if err != nil {
		return nil, err
	}
	media, err := b.Store.MediaOf(docid)
	if err != nil {
		return nil, err
	}
	cites, err := b.Store.ScripturesOf(docid)
	if err != nil {
		return nil, err
	}
	out := &WatchtowerArticle{Watchtower: *w.Watchtower, Paragraphs: []StudyParagraph{}}
	questions := map[int]string{}
	for _, blk := range parsed.Blocks {
		if blk.IsQuestion() {
			questions[blk.PID] = blk.Text()
		}
	}
	images := map[int][]Image{}
	byFile := map[string]int{}
	for i, img := range out.Images {
		byFile[img.File] = i
	}
	for _, m := range media {
		if !strings.HasPrefix(m.Mime, "image/") {
			continue
		}
		img := Image{File: m.File, Alt: content.InnerText(m.Label), Caption: content.InnerText(m.Caption), Width: m.Width, Height: m.Height, DocID: docid}
		if i, ok := byFile[m.File]; ok {
			if img.Caption == "" {
				img.Caption = out.Images[i].Caption
			}
			if img.Alt == "" {
				img.Alt = out.Images[i].Alt
			}
			out.Images[i] = img
		} else {
			byFile[m.File] = len(out.Images)
			out.Images = append(out.Images, img)
		}
		images[m.BeginPID] = append(images[m.BeginPID], img)
	}
	for _, img := range parsed.Images {
		found := false
		for _, m := range media {
			if m.File == img.File {
				found = true
				break
			}
		}
		if !found {
			images[img.PID] = append(images[img.PID], out.Images[byFile[img.File]])
		}
	}
	for _, blk := range parsed.Blocks {
		if blk.InBox || blk.Kind != content.KindPara || (blk.Num == 0 && blk.RelPID == 0) {
			continue
		}
		p := StudyParagraph{PID: blk.PID, Number: blk.Num, Text: blk.Text(), QuestionID: blk.RelPID,
			Question: questions[blk.RelPID], Scriptures: []string{}, Images: []Image{}}
		seen := map[string]bool{}
		for _, r := range append(blk.BibleRefs(), cites[blk.PID]...) {
			ref := bible.FormatList([]bible.Range{r})
			if !seen[ref] {
				p.Scriptures = append(p.Scriptures, ref)
				seen[ref] = true
			}
		}
		p.Images = append(p.Images, images[blk.PID]...)
		out.Paragraphs = append(out.Paragraphs, p)
	}
	if out.Questions == nil {
		out.Questions = []StudyQuestion{}
	}
	return out, nil
}
