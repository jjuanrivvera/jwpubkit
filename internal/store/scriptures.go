package store

import "github.com/jjuanrivvera/jwpubkit/internal/bible"

// ScripturesOf returns the citation table's references grouped by paragraph.
func (s *Store) ScripturesOf(docid int) (map[int][]bible.Range, error) {
	rows, err := s.query(`SELECT COALESCE(pid,0), first, last FROM cite WHERE docid=? ORDER BY pid, first, last`, docid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][]bible.Range{}
	for rows.Next() {
		var pid, first, last int
		if err := rows.Scan(&pid, &first, &last); err != nil {
			return nil, err
		}
		if r, ok := bible.FromIDs(first, last); ok {
			out[pid] = append(out[pid], r)
		}
	}
	return out, rows.Err()
}
