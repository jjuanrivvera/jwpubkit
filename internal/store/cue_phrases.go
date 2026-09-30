package store

import "time"

// IndexCueWindows includes adjacent cues because a sentence can straddle subtitle boundaries.
// Existing transcripts are backfilled once; each rowid is the first cue in its window.
func (s *Store) IndexCueWindows() error {
	var upper int64
	if err := s.queryRow(`SELECT COALESCE(max(rowid),0) FROM cue`).Scan(&upper); err != nil {
		return err
	}
	cursor := int64(0)
	for cursor < upper {
		var next int64
		if err := s.queryRow(`SELECT COALESCE(max(rowid),0) FROM (SELECT rowid FROM cue WHERE rowid>? AND rowid<=? ORDER BY rowid LIMIT 1000)`, cursor, upper).Scan(&next); err != nil {
			return err
		}
		if next == 0 {
			break
		}
		if _, err := s.exec(`INSERT INTO cue_window_fts(rowid,text)
 SELECT c.rowid,c.text || ' ' || COALESCE((SELECT group_concat(n.text,' ') FROM
 (SELECT text FROM cue WHERE key=c.key AND lang=c.lang AND seq>c.seq ORDER BY seq LIMIT 2) n),'')
 FROM cue c WHERE c.rowid>? AND c.rowid<=? AND NOT EXISTS(SELECT 1 FROM cue_window_fts f WHERE f.rowid=c.rowid)`, cursor, next); err != nil {
			return err
		}
		cursor = next
	}
	return nil
}

// SearchCatalogCues searches complete short windows, restricted to the discovered catalog.
func (s *Store) SearchCatalogCues(query, lang string, limit int) ([]CueHit, error) {
	fq := FTSQuery(query)
	if fq == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.query(`SELECT c.key,c.lang,c.seq,c.start_ms,c.end_ms,c.text,COALESCE(v.title,''),snippet(cue_window_fts,0,'«','»','…',24)
 FROM cue_window_fts f JOIN cue c ON c.rowid=f.rowid JOIN media_catalog m ON m.key=c.key AND m.lang=c.lang
 LEFT JOIN video v ON v.key=c.key AND v.lang=c.lang
 WHERE cue_window_fts MATCH ? AND (?='' OR c.lang=?) ORDER BY rank LIMIT ?`, fq, lang, lang, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CueHit{}
	for rows.Next() {
		var h CueHit
		var first, last int64
		if err := rows.Scan(&h.Key, &h.Lang, &h.Seq, &first, &last, &h.Text, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		h.Start = time.Duration(first) * time.Millisecond
		h.End = time.Duration(last) * time.Millisecond
		h.Seconds = int(h.Start.Seconds())
		overlap := false
		for _, old := range out {
			if old.Key == h.Key && old.Lang == h.Lang && old.Seq >= h.Seq-2 && old.Seq <= h.Seq+2 {
				overlap = true
				break
			}
		}
		if !overlap {
			out = append(out, h)
			if len(out) == limit {
				break
			}
		}
	}
	return out, rows.Err()
}
