package store

import (
	"fmt"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
)

// CatalogState records discovery separately from downloading and indexing.
func (s *Store) CatalogState(key, lang, state, checksum, message string) error {
	_, err := s.exec(`INSERT INTO media_catalog(key,lang,state,checksum,error,updated_at) VALUES(?,?,?,?,?,?)
 ON CONFLICT(key,lang) DO UPDATE SET state=excluded.state,checksum=excluded.checksum,error=excluded.error,updated_at=excluded.updated_at`, key, lang, state, checksum, message, time.Now().UTC().Format(time.RFC3339))
	return err
}

// PutCatalog keeps transcripts current when a refreshed catalog changes their URL or checksum.
func (s *Store) PutCatalog(items []cdn.MediaItem, lang string) error {
	for _, m := range items {
		key := m.LanguageAgnosticNaturalKey
		if !cdn.ValidMediaKey(key) {
			return fmt.Errorf("catalog item has an invalid language-agnostic key")
		}
		if err := s.PutVideo(key, lang, m.Title, m.Duration, m.SubtitlesURL(), m); err != nil {
			return err
		}
		state := "pending"
		if m.SubtitlesURL() == "" {
			state = "no_vtt"
		}
		_, err := s.exec(`INSERT INTO media_catalog(key,lang,state,updated_at) VALUES(?,?,?,?) ON CONFLICT(key,lang) DO NOTHING`, key, lang, state, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
	}
	return nil
}

// CatalogCoverage counts every observed state, including unavailable and failed transcripts.
func (s *Store) CatalogCoverage(lang string) (map[string]int, error) {
	rows, err := s.query(`SELECT state,count(*) FROM media_catalog WHERE lang=? GROUP BY state`, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return nil, err
		}
		out[state] = n
	}
	return out, rows.Err()
}

// CueContext returns the surrounding cues without downloading anything.
func (s *Store) CueContext(key, lang string, start, end time.Duration) ([]Cue, error) {
	rows, err := s.query(`SELECT seq,start_ms,end_ms,text FROM cue WHERE key=? AND lang=? AND end_ms>=? AND start_ms<=? ORDER BY seq`, key, lang, start.Milliseconds(), end.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Cue{}
	for rows.Next() {
		c := Cue{Key: key, Lang: lang}
		var first, last int64
		if err := rows.Scan(&c.Seq, &first, &last, &c.Text); err != nil {
			return nil, err
		}
		c.Start = time.Duration(first) * time.Millisecond
		c.End = time.Duration(last) * time.Millisecond
		c.Seconds = int(c.Start.Seconds())
		out = append(out, c)
	}
	return out, rows.Err()
}
