package cli

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
)

var docMediaKey = regexp.MustCompile(`^docid-(\d+)_(\d+)_VIDEO$`)

func (a *app) pubMediaSubtitles(key string) (string, string, error) {
	query := cdn.PubMediaQuery{Format: "MP4"}
	if match := pubKeyRe.FindStringSubmatch(key); match != nil {
		query.Pub, query.Issue = match[1], match[2]
		query.Track, _ = strconv.Atoi(match[3])
	} else if match := docMediaKey.FindStringSubmatch(key); match != nil {
		query.DocID, _ = strconv.Atoi(match[1])
		query.Track, _ = strconv.Atoi(match[2])
	} else {
		return "", "", nil
	}
	pm, err := a.client().PubMedia(a.ctx, query)
	if err != nil {
		return "", "", fmt.Errorf("pub-media subtitle lookup: %w", err)
	}
	for _, file := range pm.Files[a.lang]["MP4"] {
		if file.Subtitles != nil && file.Subtitles.URL != "" {
			return file.Subtitles.URL, file.Subtitles.Checksum, nil
		}
	}
	return "", "", nil
}
