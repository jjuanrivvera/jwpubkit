package store

import (
	"strings"

	"github.com/jjuanrivvera/jwpubkit/internal/content"
)

// PlaceSource is evidence from a publication, never an inferred map location.
type PlaceSource struct {
	Classification      string        `json:"classification"`
	ClassificationBasis string        `json:"classification_basis"`
	Kind                string        `json:"kind"`
	Source              string        `json:"source"`
	DocID               int           `json:"docid"`
	Title               string        `json:"title"`
	Publication         string        `json:"publication"`
	Key                 string        `json:"key"`
	URL                 string        `json:"url"`
	Names               []string      `json:"names"`
	Images              []PlaceFigure `json:"images"`
}

// PlaceFigure describes media without claiming a name-to-image association.
type PlaceFigure struct {
	File    string `json:"file"`
	Caption string `json:"caption"`
	Label   string `json:"label,omitempty"`
	Mime    string `json:"mime"`
}

// PlaceDocuments collects atlas citation evidence and library-wide Bible
// appendix figures. The format does not link appendix labels to chapters.
func (s *Store) PlaceDocuments(first, last int) ([]PlaceSource, error) {
	rows, err := s.query(`SELECT d.docid, COALESCE(d.title,''), p.symbol, p.key,
		CASE WHEN p.symbol='gl' THEN 'atlas_map' ELSE 'appendix_figure' END
		FROM doc d JOIN pub p ON p.id=d.pub_id
		WHERE (p.symbol='gl' AND EXISTS (
			SELECT 1 FROM cite c WHERE c.docid=d.docid AND c.pub_id=p.id AND c.first<=? AND c.last>=?))
		OR (d.class IN (14,125) AND (p.symbol='nwtsty' OR EXISTS (SELECT 1 FROM verse v WHERE v.pub_id=p.id))
			AND EXISTS (SELECT 1 FROM media m WHERE m.docid=d.docid AND m.pub_id=p.id AND m.mime='image/svg+xml'))
		ORDER BY p.symbol, p.key, d.local_id, d.docid`, last, first)
	if err != nil {
		return nil, err
	}
	out := []PlaceSource{}
	for rows.Next() {
		var p PlaceSource
		if err := rows.Scan(&p.DocID, &p.Title, &p.Publication, &p.Key, &p.Kind); err != nil {
			rows.Close()
			return nil, err
		}
		p.URL = content.DocURL(p.DocID, 0)
		p.Names, p.Images = []string{}, []PlaceFigure{}
		p.Source = "cite (chapter overlap, gl pub_id) -> doc -> media"
		if p.Kind == "appendix_figure" {
			p.Source = "doc.class (14,125) -> media.mime (image/svg+xml); par.kind (li); library-wide"
		}
		out = append(out, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// The single connection must be released before each document is enriched.
	for i := range out {
		p := &out[i]
		media, err := s.MediaOf(p.DocID)
		if err != nil {
			return nil, err
		}
		for _, m := range media {
			if !strings.HasPrefix(m.Mime, "image/") || (p.Kind == "appendix_figure" && m.Mime != "image/svg+xml") {
				continue
			}
			p.Images = append(p.Images, PlaceFigure{File: m.File, Caption: content.InnerText(m.Caption), Label: content.InnerText(m.Label), Mime: m.Mime})
		}
		if p.Kind == "appendix_figure" {
			p.Names, err = s.appendixNames(p.DocID)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s *Store) appendixNames(docid int) ([]string, error) {
	rows, err := s.query(`SELECT text FROM par WHERE docid=? AND kind='li' ORDER BY pid, id`, docid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
