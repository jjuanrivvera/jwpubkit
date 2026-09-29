// Package subs turns WebVTT subtitles into a readable transcript.
package subs

import (
	"bufio"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Cue is one subtitle.
type Cue struct {
	Start time.Duration `json:"-"`
	End   time.Duration `json:"-"`
	From  string        `json:"inicio"`
	To    string        `json:"fin"`
	Text  string        `json:"texto"`
}

var (
	timingRe = regexp.MustCompile(`^((?:\d+:)?\d{1,2}:\d{2}[.,]\d{3})\s+-->\s+((?:\d+:)?\d{1,2}:\d{2}[.,]\d{3})`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
)

// ParseVTT reads a WebVTT (or SRT) file.
func ParseVTT(src string) ([]Cue, error) {
	var cues []Cue
	sc := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(src, "\r\n", "\n")))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var cur *Cue
	var lines []string
	flush := func() {
		if cur != nil {
			cur.Text = strings.TrimSpace(strings.Join(lines, "\n"))
			if cur.Text != "" {
				cues = append(cues, *cur)
			}
		}
		cur, lines = nil, nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := timingRe.FindStringSubmatch(line); m != nil {
			flush()
			start, err1 := parseTS(m[1])
			end, err2 := parseTS(m[2])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("tiempo inválido: %q", line)
			}
			cur = &Cue{Start: start, End: end, From: FormatTS(start), To: FormatTS(end)}
			continue
		}
		if line == "" {
			flush()
			continue
		}
		if cur != nil {
			lines = append(lines, html2text(line))
		}
	}
	flush()
	if len(cues) == 0 {
		return nil, fmt.Errorf("el archivo de subtítulos no tiene texto")
	}
	return cues, sc.Err()
}

func html2text(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&nbsp;", " ", "&quot;", `"`).Replace(s)
}

func parseTS(s string) (time.Duration, error) {
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	var h, m int
	var sec float64
	var err error
	switch len(parts) {
	case 3:
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
		sec, err = strconv.ParseFloat(parts[2], 64)
	case 2:
		m, _ = strconv.Atoi(parts[0])
		sec, err = strconv.ParseFloat(parts[1], 64)
	default:
		return 0, fmt.Errorf("tiempo inválido %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second)), err
}

// FormatTS renders a duration as m:ss or h:mm:ss.
func FormatTS(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Transcript joins the cues into paragraphs: line breaks inside a cue are
// layout, not meaning, and a pause of two seconds or more starts a new
// paragraph.
func Transcript(cues []Cue) string {
	var paras []string
	var cur []string
	var lastEnd time.Duration
	for i, c := range cues {
		if i > 0 && c.Start-lastEnd >= 2*time.Second && len(cur) > 0 {
			paras = append(paras, strings.Join(cur, " "))
			cur = nil
		}
		cur = append(cur, strings.Join(strings.Fields(c.Text), " "))
		lastEnd = c.End
	}
	if len(cur) > 0 {
		paras = append(paras, strings.Join(cur, " "))
	}
	return strings.Join(paras, "\n\n")
}

// NormalizeKey accepts a mediator key ("pub-jwb-125_4_VIDEO"), a jw.org
// finder URL (…?lank=pub-jwb-125_4_VIDEO), a webpubvid link
// (webpubvid://?pub=jwb-125&track=4) or a short form ("jwb-125:4",
// "jwbai:201507:1").
func NormalizeKey(in string) (string, error) {
	in = strings.TrimSpace(in)
	switch {
	case strings.HasPrefix(in, "pub-") || strings.HasPrefix(in, "docid-"):
		if !strings.HasSuffix(in, "_VIDEO") && !strings.HasSuffix(in, "_AUDIO") {
			in += "_VIDEO"
		}
		return in, nil
	case strings.HasPrefix(in, "http"):
		u, err := url.Parse(in)
		if err != nil {
			return "", err
		}
		if lank := u.Query().Get("lank"); lank != "" {
			return lank, nil
		}
		if item := u.Query().Get("item"); item != "" {
			return item, nil
		}
		return "", fmt.Errorf("no encuentro la clave del video en %s", in)
	case strings.HasPrefix(in, "webpubvid://"):
		q, err := url.ParseQuery(strings.TrimPrefix(strings.TrimPrefix(in, "webpubvid://"), "?"))
		if err != nil {
			return "", err
		}
		track := q.Get("track")
		if d := q.Get("docid"); d != "" {
			if track == "" {
				track = "1"
			}
			return "docid-" + d + "_" + track + "_VIDEO", nil
		}
		if iss := q.Get("issue"); iss != "" && iss != "0" {
			return fmt.Sprintf("pub-%s_%s_%s_VIDEO", q.Get("pub"), strings.TrimSuffix(iss, "00"), track), nil
		}
		return fmt.Sprintf("pub-%s_%s_VIDEO", q.Get("pub"), track), nil
	case strings.Count(in, ":") == 1:
		p, t, _ := strings.Cut(in, ":")
		return fmt.Sprintf("pub-%s_%s_VIDEO", p, t), nil
	case strings.Count(in, ":") == 2:
		parts := strings.Split(in, ":")
		return fmt.Sprintf("pub-%s_%s_%s_VIDEO", parts[0], parts[1], parts[2]), nil
	}
	return "", fmt.Errorf("clave de video no reconocida: %q (ejemplo: pub-jwb-125_4_VIDEO)", in)
}
