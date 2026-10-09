package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

var tableOrder = []string{"Location", "IndependentMedia", "PlaylistItemAccuracy", "UserMark", "BlockRange", "Note", "Tag", "PlaylistItem", "PlaylistItemIndependentMediaMap", "PlaylistItemLocationMap", "PlaylistItemMarker", "PlaylistItemMarkerBibleVerseMap", "PlaylistItemMarkerParagraphMap", "Bookmark", "InputField", "TagMap"}

type row map[string]any
type source struct {
	name     string
	a        *Archive
	rows     map[string][]row
	ids      map[string]map[any]any
	paths    map[string]string
	dateSpan *[2]time.Time
}

type Conflict struct {
	Table      string   `json:"table"`
	Key        string   `json:"key"`
	Winner     string   `json:"winner"`
	Other      string   `json:"other"`
	Fields     []string `json:"fields"`
	Resolution string   `json:"resolution"`
	Kind       string   `json:"kind,omitempty"`
}
type Overlap struct {
	Source     string `json:"source"`
	Guid       string `json:"guid"`
	Location   int64  `json:"location"`
	BlockType  int64  `json:"block_type"`
	Identifier int64  `json:"identifier"`
	Start      any    `json:"start_token"`
	End        any    `json:"end_token"`
	KeptGuid   string `json:"kept_guid"`
}
type Report struct {
	Before        map[string]map[string]int    `json:"before"`
	After         map[string]int               `json:"after"`
	Added         map[string]map[string]int    `json:"added"`
	Conflicts     []Conflict                   `json:"conflicts"`
	Overlaps      []Overlap                    `json:"overlaps"`
	RenamedMedia  map[string]map[string]string `json:"renamed_media"`
	Device        string                       `json:"device_name"`
	DryRun        bool                         `json:"dry_run"`
	Base          string                       `json:"base,omitempty"`
	BaseCounts    map[string]int               `json:"base_counts,omitempty"`
	Deletions     []Deletion                   `json:"deletions,omitempty"`
	DeletedCounts map[string]int               `json:"deleted_counts,omitempty"`
}

type MergeOptions struct {
	Base        string
	Prefer      string
	TablePrefer map[string]string
	Device      string
	DryRun      bool
	Resolve     Resolver
	sourceTimes map[string][2]time.Time
}

// Resolver receives private text only when a caller explicitly requests interactive review.
// Three-way versions carry _source, which must survive the decision, and deleted
// versions carry _deleted=true. Returning nil retains the configured preference.
type Resolver func(Conflict, map[string]any, map[string]any) (map[string]any, error)

func readRows(ctx context.Context, db *sql.DB, table string) ([]row, error) {
	query := "SELECT * FROM " + quote(table)
	if table == "TagMap" {
		query += " ORDER BY TagId, Position"
	}
	r, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cols, err := r.Columns()
	if err != nil {
		return nil, err
	}
	var out []row
	for r.Next() {
		v := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range v {
			ptr[i] = &v[i]
		}
		if err = r.Scan(ptr...); err != nil {
			return nil, err
		}
		m := row{}
		for i, c := range cols {
			if b, ok := v[i].([]byte); ok {
				v[i] = string(b)
			}
			m[c] = v[i]
		}
		out = append(out, m)
	}
	return out, r.Err()
}

func copyRow(r row) row {
	m := row{}
	for k, v := range r {
		m[k] = v
	}
	return m
}
func key(r row, cols ...string) string {
	v := make([]any, 0, len(cols))
	for _, c := range cols {
		v = append(v, r[c])
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func locationKey(r row) string {
	// Bible identities use the chapter key even when a document id is also present.
	if n, ok := r["BookNumber"].(int64); ok && n != 0 {
		return "bible:" + key(r, "BookNumber", "ChapterNumber", "KeySymbol", "MepsLanguage", "Type")
	}
	m := copyRow(r)
	for _, c := range []string{"Specialty", "Edition"} {
		if m[c] == nil {
			m[c] = ""
		}
	}
	return "media:" + key(m, "KeySymbol", "IssueTagNumber", "MepsLanguage", "DocumentId", "Track", "Type", "Specialty", "Edition")
}

func tableKey(t string, r row) string {
	switch t {
	case "Location":
		return locationKey(r)
	case "UserMark":
		return key(r, "UserMarkGuid")
	case "Note":
		return key(r, "Guid")
	case "Tag":
		return key(r, "Type", "Name")
	case "IndependentMedia":
		return key(r, "FilePath")
	case "PlaylistItemAccuracy":
		return key(r, "Description")
	case "Bookmark":
		return key(r, "PublicationLocationId", "Slot")
	case "InputField":
		return key(r, "LocationId", "TextTag")
	case "TagMap":
		return key(r, "TagId", "NoteId", "LocationId", "PlaylistItemId")
	case "PlaylistItemIndependentMediaMap":
		return key(r, "PlaylistItemId", "IndependentMediaId")
	case "PlaylistItemLocationMap":
		return key(r, "PlaylistItemId", "LocationId")
	case "PlaylistItemMarker":
		return key(r, "PlaylistItemId", "StartTimeTicks")
	case "PlaylistItemMarkerBibleVerseMap":
		return key(r, "PlaylistItemMarkerId", "VerseId")
	case "PlaylistItemMarkerParagraphMap":
		return key(r, "PlaylistItemMarkerId", "MepsDocumentId", "ParagraphIndex", "MarkerIndexWithinParagraph")
	default:
		return ""
	}
}

func scalarID(t string) string {
	switch t {
	case "InputField", "PlaylistItemIndependentMediaMap", "PlaylistItemLocationMap", "PlaylistItemMarkerBibleVerseMap", "PlaylistItemMarkerParagraphMap":
		return ""
	default:
		return t + "Id"
	}
}

func insert(ctx context.Context, tx *sql.Tx, t string, r row) error {
	cols := make([]string, 0, len(r))
	for c := range r {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	names := make([]string, len(cols))
	args := make([]any, len(cols))
	marks := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quote(c)
		args[i] = r[c]
		marks[i] = "?"
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO "+quote(t)+" ("+strings.Join(names, ",")+") VALUES ("+strings.Join(marks, ",")+")", args...)
	return err
}

func ordered(src []*source, prefer string) ([]*source, error) {
	out := append([]*source(nil), src...)
	if prefer == "" {
		return out, nil
	}
	if prefer == "newest" || prefer == "oldest" {
		for _, s := range out {
			if _, err := time.Parse(time.RFC3339, s.a.Manifest.Backup.LastModified); err != nil {
				return nil, fmt.Errorf("invalid lastModifiedDate in %s", s.name)
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			a := sourceTime(out[i], prefer)
			b := sourceTime(out[j], prefer)
			if prefer == "newest" {
				return a.After(b)
			}
			return a.Before(b)
		})
		return out, nil
	}
	p, err := filepath.Abs(prefer)
	if err != nil {
		return nil, err
	}
	for i, s := range out {
		if s.name == p {
			out = append([]*source{s}, append(out[:i], out[i+1:]...)...)
			return out, nil
		}
	}
	return nil, fmt.Errorf("preferred file %q is not an input", prefer)
}

func Merge(ctx context.Context, inputs []string, output string, opts MergeOptions) (*Report, error) {
	if len(inputs) < 2 {
		return nil, errors.New("merge requires at least two backups")
	}
	if opts.Base != "" && len(inputs) != 2 {
		return nil, errors.New("--base requires exactly two side backups")
	}
	r := &Report{Before: map[string]map[string]int{}, Added: map[string]map[string]int{}, RenamedMedia: map[string]map[string]string{}, DryRun: opts.DryRun, Conflicts: []Conflict{}, Overlaps: []Overlap{}}
	var src []*source
	defer func() {
		for _, s := range src {
			s.a.Close()
		}
	}()
	for _, name := range inputs {
		abs, e := filepath.Abs(name)
		if e != nil {
			return nil, e
		}
		if _, ok := r.Before[abs]; ok {
			return nil, errors.New("duplicate input file")
		}
		a, e := Open(ctx, abs)
		if e != nil {
			return nil, fmt.Errorf("%s: %w", name, e)
		}
		s := &source{name: abs, a: a, rows: map[string][]row{}, ids: map[string]map[any]any{}, paths: map[string]string{}}
		if span, ok := opts.sourceTimes[abs]; ok {
			s.dateSpan = &span
		}
		src = append(src, s)
		i, e := a.Inspect(ctx)
		if e != nil {
			return nil, e
		}
		r.Before[abs] = i.Counts
		r.Added[abs] = map[string]int{}
		for _, t := range tableOrder {
			s.rows[t], e = readRows(ctx, a.DB, t)
			if e != nil {
				return nil, e
			}
			s.ids[t] = map[any]any{}
		}
	}
	src, err := ordered(src, opts.Prefer)
	if err != nil {
		return nil, err
	}
	a := src[0].a
	schema, err := readRows(ctx, a.DB, "sqlite_master")
	if err != nil {
		return nil, err
	}
	for _, s := range src[1:] {
		other, e := readRows(ctx, s.a.DB, "sqlite_master")
		if e != nil {
			return nil, e
		}
		if !sameSchema(schema, other) {
			return nil, errors.New("input schemas differ; refusing a lossy merge")
		}
	}
	var plan *threeWayPlan
	if opts.Base != "" {
		ancestor, e := loadSource(ctx, opts.Base)
		if e != nil {
			return nil, fmt.Errorf("ancestor: %w", e)
		}
		defer ancestor.a.Close()
		baseSchema, e := readRows(ctx, ancestor.a.DB, "sqlite_master")
		if e != nil {
			return nil, e
		}
		if !sameSchema(schema, baseSchema) {
			return nil, errors.New("ancestor schema differs from the sides")
		}
		i, e := ancestor.a.Inspect(ctx)
		if e != nil {
			return nil, e
		}
		r.Base = ancestor.name
		r.BaseCounts = i.Counts
		r.DeletedCounts = map[string]int{}
		plan, e = planThreeWay(src, ancestor, opts, r)
		if e != nil {
			return nil, e
		}
	}
	// File names are not identities: rename collisions before remapping media references.
	for _, s := range src {
		r.RenamedMedia[s.name] = map[string]string{}
		for name, b := range s.a.Files {
			if name == "manifest.json" || name == "userData.db" {
				continue
			}
			dest := name
			if old, ok := a.Files[dest]; ok && !bytes.Equal(old, b) {
				h := sha256.Sum256(b)
				dest = fmt.Sprintf("merged-%x-%s", h[:8], path.Base(name))
				if old, ok = a.Files[dest]; ok && !bytes.Equal(old, b) {
					return nil, errors.New("media hash collision")
				}
				r.RenamedMedia[s.name][name] = dest
			}
			a.Files[dest] = b
			s.paths[name] = dest
		}
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, s := range schema {
		if s["type"] == "trigger" {
			if _, err = tx.ExecContext(ctx, "DROP TRIGGER "+quote(s["name"].(string))); err != nil {
				return nil, err
			}
		}
	}
	for i := len(tableOrder) - 1; i >= 0; i-- {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+quote(tableOrder[i])); err != nil {
			return nil, err
		}
	}
	m := merger{ctx: ctx, tx: tx, report: r, accepted: map[string][]acceptedRow{}, next: map[string]int64{}, resolve: opts.Resolve, playlistKeys: map[string]acceptedRow{}, threeWay: plan != nil}
	if plan != nil {
		plan.reserveAncestorIDs(&m)
	}
	for _, t := range tableOrder {
		if t == "BlockRange" {
			continue
		}
		prefer := opts.Prefer
		if p := opts.TablePrefer[t]; p != "" {
			prefer = p
		}
		order, e := ordered(src, prefer)
		if e != nil {
			return nil, e
		}
		for _, s := range order {
			for _, original := range s.rows[t] {
				v := copyRow(original)
				if err = m.remap(s, t, v); err != nil {
					return nil, err
				}
				if err = m.add(s, t, original, v); err != nil {
					return nil, err
				}
			}
		}
		if plan != nil {
			plan.bindAliases(t)
		}
	}
	for _, s := range schema {
		if s["type"] == "trigger" {
			if _, err = tx.ExecContext(ctx, s["sql"].(string)); err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if err = a.Validate(ctx); err != nil {
		return nil, err
	}
	i, err := a.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	r.After = i.Counts
	r.Device = opts.Device
	if r.Device == "" {
		r.Device = a.Manifest.Backup.Device + " (merged)"
	}
	if !opts.DryRun {
		if output == "" {
			return nil, errors.New("output is required unless --dry-run is set")
		}
		if err = a.Save(ctx, output, r.Device); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func sameSchema(a, b []row) bool {
	f := func(rows []row) map[string]any {
		m := map[string]any{}
		for _, r := range rows {
			if r["sql"] != nil {
				m[key(r, "type", "name")] = r["sql"]
			}
		}
		return m
	}
	return reflect.DeepEqual(f(a), f(b))
}

type acceptedRow struct {
	r      row
	source string
}
type merger struct {
	ctx          context.Context
	tx           *sql.Tx
	report       *Report
	accepted     map[string][]acceptedRow
	next         map[string]int64
	resolve      Resolver
	playlistKeys map[string]acceptedRow
	threeWay     bool
}

func (m *merger) remap(s *source, t string, v row) error {
	for col, val := range v {
		if val == nil || col == scalarID(t) {
			continue
		}
		parent := ""
		switch col {
		case "LocationId", "PublicationLocationId":
			parent = "Location"
		case "UserMarkId":
			parent = "UserMark"
		case "NoteId":
			parent = "Note"
		case "TagId":
			parent = "Tag"
		case "PlaylistItemId":
			parent = "PlaylistItem"
		case "PlaylistItemMarkerId":
			parent = "PlaylistItemMarker"
		case "IndependentMediaId":
			parent = "IndependentMedia"
		case "Accuracy":
			parent = "PlaylistItemAccuracy"
		}
		if parent != "" {
			mapped, ok := s.ids[parent][val]
			if !ok {
				return fmt.Errorf("unmapped %s.%s", t, col)
			}
			v[col] = mapped
		}
		if col == "FilePath" || col == "ThumbnailFilePath" {
			dest, ok := s.paths[val.(string)]
			if !ok {
				return fmt.Errorf("missing media %q", val)
			}
			v[col] = dest
		}
	}
	return nil
}

func (m *merger) add(s *source, t string, original, v row) error {
	id := scalarID(t)
	k := tableKey(t, v)
	if t == "PlaylistItem" && !m.threeWay {
		signature, err := m.playlistKey(s, original, v)
		if err != nil {
			return err
		}
		k = signature
		if held, ok := m.playlistKeys[k]; ok && held.source != s.name {
			s.ids[t][original[id]] = held.r[id]
			return nil
		}
	}
	for i, held := range m.accepted[t] {
		if k != "" && tableKey(t, held.r) == k {
			if id != "" {
				s.ids[t][original[id]] = held.r[id]
			}
			a, b := copyRow(held.r), copyRow(v)
			delete(a, id)
			delete(b, id)
			if t == "TagMap" {
				delete(a, "Position")
				delete(b, "Position")
			}
			if !reflect.DeepEqual(a, b) {
				var fields []string
				for field, value := range a {
					if !reflect.DeepEqual(value, b[field]) {
						fields = append(fields, field)
					}
				}
				sort.Strings(fields)
				c := Conflict{Table: t, Key: k, Winner: held.source, Other: s.name, Fields: fields, Resolution: "preferred"}
				textDiff := !reflect.DeepEqual(a["Content"], b["Content"]) || !reflect.DeepEqual(a["Title"], b["Title"]) || !reflect.DeepEqual(a["Value"], b["Value"])
				if m.resolve != nil && textDiff && (t == "Note" || t == "InputField") {
					chosen, err := m.resolve(c, copyRow(held.r), copyRow(v))
					if err != nil {
						return err
					}
					if chosen != nil {
						clean := copyRow(chosen)
						delete(clean, id)
						switch {
						case reflect.DeepEqual(clean, a):
							c.Resolution = "preferred"
						case reflect.DeepEqual(clean, b):
							c.Resolution = "other"
							m.accepted[t][i].source = s.name
						default:
							c.Resolution = "merged_or_edited"
						}
						chosen[id] = held.r[id]
						if id == "" {
							delete(chosen, "")
						}
						if err = m.update(t, held.r, chosen); err != nil {
							return err
						}
						m.accepted[t][i].r = chosen
					}
				}
				m.report.Conflicts = append(m.report.Conflicts, c)
			}
			if t == "UserMark" {
				return m.compareRanges(s, original, held)
			}
			return nil
		}
	}
	if t == "UserMark" {
		return m.addMark(s, original, v)
	}
	if id != "" {
		planned, ok := s.ids[t][original[id]]
		if m.threeWay && ok {
			v[id] = planned
		} else {
			m.next[t]++
			v[id] = m.next[t]
		}
		s.ids[t][original[id]] = v[id]
	}
	if t == "TagMap" {
		var pos int64 = -1
		otherSource := false
		for _, h := range m.accepted[t] {
			if h.r["TagId"] == v["TagId"] {
				if h.source != s.name {
					otherSource = true
				}
				if h.r["Position"].(int64) > pos {
					pos = h.r["Position"].(int64)
				}
			}
		}
		if otherSource {
			v["Position"] = pos + 1
		}
	}
	if err := insert(m.ctx, m.tx, t, v); err != nil {
		return fmt.Errorf("%s: %w", t, err)
	}
	m.accepted[t] = append(m.accepted[t], acceptedRow{v, s.name})
	m.report.Added[s.name][t]++
	return nil
}

func (m *merger) update(t string, held, v row) error {
	var cols []string
	for c := range v {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	var parts []string
	var args []any
	for _, c := range cols {
		parts = append(parts, quote(c)+"=?")
		args = append(args, v[c])
	}
	where := "NoteId=?"
	args = append(args, held["NoteId"])
	if t == "InputField" {
		where = "LocationId=? AND TextTag=?"
		args = args[:len(args)-1]
		args = append(args, held["LocationId"], held["TextTag"])
	}
	_, err := m.tx.ExecContext(m.ctx, "UPDATE "+quote(t)+" SET "+strings.Join(parts, ",")+" WHERE "+where, args...)
	return err
}

func (m *merger) playlistKey(s *source, original, v row) (string, error) {
	item := copyRow(v)
	delete(item, "PlaylistItemId")
	var children []string
	for _, t := range []string{"PlaylistItemIndependentMediaMap", "PlaylistItemLocationMap", "PlaylistItemMarker"} {
		for _, r := range s.rows[t] {
			if r["PlaylistItemId"] != original["PlaylistItemId"] {
				continue
			}
			c := copyRow(r)
			delete(c, "PlaylistItemId")
			delete(c, "PlaylistItemMarkerId")
			if err := m.remap(s, t, c); err != nil {
				return "", err
			}
			b, _ := json.Marshal(c)
			children = append(children, t+string(b))
			if t == "PlaylistItemMarker" {
				for _, mt := range []string{"PlaylistItemMarkerBibleVerseMap", "PlaylistItemMarkerParagraphMap"} {
					for _, mr := range s.rows[mt] {
						if mr["PlaylistItemMarkerId"] == r["PlaylistItemMarkerId"] {
							mc := copyRow(mr)
							delete(mc, "PlaylistItemMarkerId")
							mb, _ := json.Marshal(mc)
							children = append(children, mt+string(mb)+key(r, "StartTimeTicks"))
						}
					}
				}
			}
		}
	}
	sort.Strings(children)
	b, _ := json.Marshal([]any{item, children})
	signature := string(b)
	// Store the item after its new id has been assigned, using the predicted next id.
	if _, ok := m.playlistKeys[signature]; !ok {
		held := copyRow(v)
		held["PlaylistItemId"] = m.next["PlaylistItem"] + 1
		m.playlistKeys[signature] = acceptedRow{held, s.name}
	}
	return signature, nil
}

func (m *merger) compareRanges(s *source, original row, held acceptedRow) error {
	var a, b []string
	for _, v := range s.rows["BlockRange"] {
		if v["UserMarkId"] == original["UserMarkId"] {
			a = append(a, key(v, "BlockType", "Identifier", "StartToken", "EndToken"))
		}
	}
	for _, v := range m.accepted["BlockRange"] {
		if v.r["UserMarkId"] == held.r["UserMarkId"] {
			b = append(b, key(v.r, "BlockType", "Identifier", "StartToken", "EndToken"))
		}
	}
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		m.report.Conflicts = append(m.report.Conflicts, Conflict{Table: "BlockRange", Key: key(original, "UserMarkGuid"), Winner: held.source, Other: s.name, Fields: []string{"ranges"}, Resolution: "preferred"})
	}
	return nil
}

func (m *merger) addMark(s *source, original, v row) error {
	var kept []row
	var fallback any
	for _, br := range s.rows["BlockRange"] {
		if br["UserMarkId"] != original["UserMarkId"] {
			continue
		}
		fragments := []row{copyRow(br)}
		for _, h := range m.accepted["BlockRange"] {
			if h.source == s.name || h.r["BlockType"] != br["BlockType"] || h.r["Identifier"] != br["Identifier"] {
				continue
			}
			var mark row
			for _, um := range m.accepted["UserMark"] {
				if um.r["UserMarkId"] == h.r["UserMarkId"] {
					mark = um.r
					break
				}
			}
			if mark["LocationId"] != v["LocationId"] {
				continue
			}
			var next []row
			for _, f := range fragments {
				parts, overlap := subtract(f, h.r)
				if overlap {
					fallback = mark["UserMarkId"]
					m.report.Overlaps = append(m.report.Overlaps, Overlap{s.name, v["UserMarkGuid"].(string), v["LocationId"].(int64), br["BlockType"].(int64), br["Identifier"].(int64), f["StartToken"], f["EndToken"], mark["UserMarkGuid"].(string)})
				}
				next = append(next, parts...)
			}
			fragments = next
		}
		kept = append(kept, fragments...)
	}
	if len(kept) == 0 && fallback != nil {
		s.ids["UserMark"][original["UserMarkId"]] = fallback
		return nil
	}
	m.next["UserMark"]++
	v["UserMarkId"] = m.next["UserMark"]
	s.ids["UserMark"][original["UserMarkId"]] = v["UserMarkId"]
	if err := insert(m.ctx, m.tx, "UserMark", v); err != nil {
		return err
	}
	m.accepted["UserMark"] = append(m.accepted["UserMark"], acceptedRow{v, s.name})
	m.report.Added[s.name]["UserMark"]++
	for _, f := range kept {
		m.next["BlockRange"]++
		f["BlockRangeId"] = m.next["BlockRange"]
		f["UserMarkId"] = v["UserMarkId"]
		if err := insert(m.ctx, m.tx, "BlockRange", f); err != nil {
			return err
		}
		m.accepted["BlockRange"] = append(m.accepted["BlockRange"], acceptedRow{f, s.name})
		m.report.Added[s.name]["BlockRange"]++
	}
	return nil
}

// NULL boundaries denote an entire block; numeric boundaries are inclusive.
func subtract(a, b row) ([]row, bool) {
	if b["StartToken"] == nil || b["EndToken"] == nil {
		return nil, true
	}
	if a["StartToken"] == nil || a["EndToken"] == nil {
		// A partial range cannot bound the remainder of a whole-block mark. Keep the winner.
		return nil, true
	}
	as, ae := a["StartToken"].(int64), a["EndToken"].(int64)
	bs, be := b["StartToken"].(int64), b["EndToken"].(int64)
	if ae < bs || be < as {
		return []row{a}, false
	}
	var out []row
	if as < bs {
		r := copyRow(a)
		r["EndToken"] = bs - 1
		out = append(out, r)
	}
	if ae > be {
		r := copyRow(a)
		r["StartToken"] = be + 1
		out = append(out, r)
	}
	return out, true
}

// Intermediate sync archives have a new save date, which must not outweigh the
// original input dates when resolving conflicts later in the same batch.
func sourceTime(s *source, prefer string) time.Time {
	if s.dateSpan != nil {
		if prefer == "oldest" {
			return s.dateSpan[0]
		}
		return s.dateSpan[1]
	}
	stamp, _ := time.Parse(time.RFC3339, s.a.Manifest.Backup.LastModified)
	return stamp
}
