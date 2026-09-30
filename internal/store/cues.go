package store

import (
	"database/sql"
	"strings"
	"time"
)

// Cue is one subtitle of a video, with where it starts.
type Cue struct {
	Key   string        `json:"key"`
	Lang  string        `json:"language"`
	Seq   int           `json:"seq"`
	Start time.Duration `json:"-"`
	End   time.Duration `json:"-"`
	Text  string        `json:"text"`
	// Seconds is the start of the cue in whole seconds, which is what a player
	// needs and what a person writes down.
	Seconds int `json:"seconds"`
}

// PutCues records a video's transcript so it can be searched later. The whole
// transcript is replaced, because a re-fetch is the only reason to write it.
func (s *Store) PutCues(key, lang string, starts, ends []time.Duration, texts []string) error {
	tx, err := s.begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only if Commit did not happen

	if _, err := tx.Exec(`DELETE FROM cue_fts WHERE rowid IN (
		SELECT rowid_ FROM cue_map WHERE key=? AND lang=?)`, key, lang); err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM cue_map WHERE key=? AND lang=?`, `DELETE FROM cue WHERE key=? AND lang=?`} {
		if _, err := tx.Exec(q, key, lang); err != nil {
			return err
		}
	}
	insCue, err := tx.Prepare(`INSERT INTO cue(key, lang, seq, start_ms, end_ms, text) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insCue.Close()
	insFTS, err := tx.Prepare(`INSERT INTO cue_fts(text) VALUES(?)`)
	if err != nil {
		return err
	}
	defer insFTS.Close()
	insMap, err := tx.Prepare(`INSERT INTO cue_map(rowid_, key, lang, seq) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insMap.Close()

	for i, text := range texts {
		if strings.TrimSpace(text) == "" {
			continue
		}
		if _, err := insCue.Exec(key, lang, i, starts[i].Milliseconds(), ends[i].Milliseconds(), text); err != nil {
			return err
		}
		res, err := insFTS.Exec(text)
		if err != nil {
			return err
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := insMap.Exec(rowid, key, lang, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HasCues reports whether a video's transcript is already recorded.
func (s *Store) HasCues(key, lang string) bool {
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM cue WHERE key=? AND lang=?`, key, lang).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// CueHit is a phrase found in a video, at the second it is said.
type CueHit struct {
	Cue
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// SearchCues finds a phrase across every transcript in the library. The same
// query syntax as the rest of the tool: quoted phrases, prefix*, accents ignored.
func (s *Store) SearchCues(query, lang string, limit int) ([]CueHit, error) {
	fq := FTSQuery(query)
	if fq == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	args := []any{fq}
	langFilter := ""
	if lang != "" {
		langFilter = ` AND m.lang = ?`
		args = append(args, lang)
	}
	args = append(args, limit)
	rows, err := s.DB.Query(`
		SELECT c.key, c.lang, c.seq, c.start_ms, c.end_ms, c.text,
		       COALESCE(v.title,''), snippet(cue_fts, 0, '«', '»', '…', 12)
		FROM cue_fts f
		JOIN cue_map m ON m.rowid_ = f.rowid
		JOIN cue c ON c.key = m.key AND c.lang = m.lang AND c.seq = m.seq
		LEFT JOIN video v ON v.key = c.key AND v.lang = c.lang
		WHERE cue_fts MATCH ?`+langFilter+`
		ORDER BY rank LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CueHit
	for rows.Next() {
		var h CueHit
		var startMS, endMS int64
		if err := rows.Scan(&h.Key, &h.Lang, &h.Seq, &startMS, &endMS, &h.Text, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		h.Start = time.Duration(startMS) * time.Millisecond
		h.End = time.Duration(endMS) * time.Millisecond
		h.Seconds = int(h.Start.Seconds())
		out = append(out, h)
	}
	return out, rows.Err()
}

// CuesOf returns a video's whole transcript in order.
func (s *Store) CuesOf(key, lang string) ([]Cue, error) {
	rows, err := s.DB.Query(`SELECT seq, start_ms, end_ms, text FROM cue
		WHERE key=? AND lang=? ORDER BY seq`, key, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cue
	for rows.Next() {
		c := Cue{Key: key, Lang: lang}
		var startMS, endMS int64
		if err := rows.Scan(&c.Seq, &startMS, &endMS, &c.Text); err != nil {
			return nil, err
		}
		c.Start = time.Duration(startMS) * time.Millisecond
		c.End = time.Duration(endMS) * time.Millisecond
		c.Seconds = int(c.Start.Seconds())
		out = append(out, c)
	}
	return out, rows.Err()
}

// CueVideoCount is how many videos have a transcript recorded, which is what
// makes an empty search result explainable rather than mysterious.
func (s *Store) CueVideoCount(lang string) (int, error) {
	q := `SELECT count(DISTINCT key) FROM cue`
	var args []any
	if lang != "" {
		q += ` WHERE lang = ?`
		args = append(args, lang)
	}
	var n int
	err := s.DB.QueryRow(q, args...).Scan(&n)
	return n, err
}

var _ = sql.ErrNoRows
