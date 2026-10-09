package backup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
)

// Deletion reports identities and outcomes without exposing annotation text.
type Deletion struct {
	Table      string   `json:"table"`
	Key        string   `json:"key"`
	DeletedIn  []string `json:"deleted_in"`
	Reason     string   `json:"reason"`
	Resolution string   `json:"resolution"`
}

type snapshot struct {
	s          *source
	entries    map[string]map[string]row
	identities map[string]map[any]string
}
type selection struct {
	snap *snapshot
	r    row
}
type threeWayPlan struct {
	sides    []*snapshot
	base     *snapshot
	selected map[string]map[string]selection
}

func loadSource(ctx context.Context, name string) (*source, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	a, err := Open(ctx, abs)
	if err != nil {
		return nil, err
	}
	s := &source{name: abs, a: a, rows: map[string][]row{}, ids: map[string]map[any]any{}, paths: map[string]string{}}
	for _, t := range tableOrder {
		s.rows[t], err = readRows(ctx, a.DB, t)
		if err != nil {
			a.Close()
			return nil, err
		}
		s.ids[t] = map[any]any{}
	}
	return s, nil
}

func foreignTable(col string) string {
	switch col {
	case "LocationId", "PublicationLocationId":
		return "Location"
	case "UserMarkId":
		return "UserMark"
	case "NoteId":
		return "Note"
	case "TagId":
		return "Tag"
	case "PlaylistItemId":
		return "PlaylistItem"
	case "PlaylistItemMarkerId":
		return "PlaylistItemMarker"
	case "IndependentMediaId":
		return "IndependentMedia"
	case "Accuracy":
		return "PlaylistItemAccuracy"
	}
	return ""
}

func (v *snapshot) canonical(t string, r row) row {
	out := copyRow(r)
	delete(out, scalarID(t))
	for c, value := range out {
		parent := foreignTable(c)
		if parent != "" && value != nil {
			out[c] = v.identities[parent][value]
		}
	}
	return out
}

func newSnapshot(s *source, base *snapshot) *snapshot {
	v := &snapshot{s: s, entries: map[string]map[string]row{}, identities: map[string]map[any]string{}}
	for _, t := range tableOrder {
		v.entries[t] = map[string]row{}
		v.identities[t] = map[any]string{}
	}
	for _, t := range tableOrder {
		if t == "BlockRange" {
			continue
		}
		for _, r := range s.rows[t] {
			c := v.canonical(t, r)
			k := tableKey(t, c)
			if t == "IndependentMedia" {
				k = key(c, "FilePath", "Hash")
			}
			if t == "Tag" || t == "PlaylistItem" || t == "PlaylistItemMarker" {
				if t == "PlaylistItem" {
					k = "item:" + key(c, "Label", "StartTrimOffsetTicks", "EndTrimOffsetTicks", "Accuracy", "EndAction", "ThumbnailFilePath")
				}
				if base == nil {
					k = fmt.Sprintf("ancestor:%s:%v", t, r[scalarID(t)])
				} else {
					matched := ""
					for bk, br := range base.entries[t] {
						if reflect.DeepEqual(base.canonical(t, br), c) {
							matched = bk
							break
						}
					}
					// GUID-less rows retain their ancestor IDs on normal JW Library descendants.
					// Prefer semantic matches so an unchanged renumbered row is not mistaken for a deletion.
					if matched == "" {
						matched = base.identities[t][r[scalarID(t)]]
					}
					if matched != "" {
						k = matched
					}
				}
			}
			if _, ok := v.entries[t][k]; ok {
				k = fmt.Sprintf("%s:instance:%v", k, r[scalarID(t)])
			}
			v.entries[t][k] = r
			if id := scalarID(t); id != "" {
				v.identities[t][r[id]] = k
			}
		}
	}
	return v
}

func (v *snapshot) comparable(t string, r row, dependents bool) row {
	out := v.canonical(t, r)
	if t == "UserMark" {
		var ranges []string
		for _, b := range v.s.rows["BlockRange"] {
			if b["UserMarkId"] == r["UserMarkId"] {
				ranges = append(ranges, key(b, "BlockType", "Identifier", "StartToken", "EndToken"))
			}
		}
		sort.Strings(ranges)
		out["ranges"] = ranges
	}
	if dependents {
		id := scalarID(t)
		children := map[string][]string{}
		for _, child := range tableOrder {
			if child == "BlockRange" {
				continue
			}
			for _, cr := range v.s.rows[child] {
				if child == t || cr[id] != r[id] || foreignTable(id) != t {
					continue
				}
				canonical := v.comparable(child, cr, child == "PlaylistItemMarker")
				children[child] = append(children[child], key(row{"row": canonical}, "row"))
			}
		}
		for child, rows := range children {
			sort.Strings(rows)
			children[child] = rows
		}
		out["children"] = children
	}
	return out
}

func planThreeWay(src []*source, ancestor *source, opts MergeOptions, report *Report) (*threeWayPlan, error) {
	p := &threeWayPlan{base: newSnapshot(ancestor, nil), selected: map[string]map[string]selection{}}
	for _, s := range src {
		p.sides = append(p.sides, newSnapshot(s, p.base))
	}
	for _, t := range tableOrder {
		if t == "BlockRange" {
			continue
		}
		p.selected[t] = map[string]selection{}
		if t == "Location" || t == "IndependentMedia" || t == "PlaylistItemAccuracy" {
			continue
		}
		pref := opts.Prefer
		if v := opts.TablePrefer[t]; v != "" {
			pref = v
		}
		order, err := ordered(src, pref)
		if err != nil {
			return nil, err
		}
		first, second := p.sides[0], p.sides[1]
		if order[0] != first.s {
			first, second = second, first
		}
		keys := map[string]bool{}
		for _, v := range []*snapshot{p.base, first, second} {
			for k := range v.entries[t] {
				keys[k] = true
			}
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b, a, z := p.base.entries[t][k], first.entries[t][k], second.entries[t][k]
			var chosen selection
			var e error
			switch {
			case p.restoreCascaded(t, b, a, first, second, z):
				chosen = selection{second, z}
			case p.restoreCascaded(t, b, z, second, first, a):
				chosen = selection{first, a}
			default:
				chosen, e = p.decide(t, k, b, a, z, first, second, opts, report)
			}
			if e != nil {
				return nil, e
			}
			if chosen.r != nil && !p.parentsSurvive(t, chosen.snap, chosen.r) {
				chosen = selection{}
				p.recordDelete(t, k, b, []string{}, "parent_deleted", report)
			}
			if chosen.r != nil {
				p.selected[t][k] = chosen
			}
		}
	}
	p.reportRangeDeletions(report)
	for _, v := range p.sides {
		for t, rows := range p.selected {
			if t == "Location" || t == "IndependentMedia" || t == "PlaylistItemAccuracy" {
				continue
			}
			v.s.rows[t] = nil
			for _, k := range sortedSelections(rows) {
				s := rows[k]
				if s.snap == v {
					r := copyRow(s.r)
					if t == "Note" && r["UserMarkId"] != nil {
						mark := v.identities["UserMark"][r["UserMarkId"]]
						if _, ok := p.selected["UserMark"][mark]; !ok {
							r["UserMarkId"] = nil
						}
					}
					v.s.rows[t] = append(v.s.rows[t], r)
				}
			}
		}
		kept := []row{}
		for _, r := range v.s.rows["BlockRange"] {
			k := v.identities["UserMark"][r["UserMarkId"]]
			if s, ok := p.selected["UserMark"][k]; ok && s.snap == v {
				kept = append(kept, r)
			}
		}
		v.s.rows["BlockRange"] = kept
	}
	return p, nil
}

func sortedSelections(rows map[string]selection) []string {
	keys := make([]string, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// A restored parent also restores children removed implicitly by its cascade.
// Directly removed children, whose parent still exists on that side, remain deletions.
func (p *threeWayPlan) restoreCascaded(t string, base, missing row, side, other *snapshot, survivor row) bool {
	if base == nil || missing != nil || survivor == nil {
		return false
	}
	for col, value := range survivor {
		parent := foreignTable(col)
		if parent == "" || col == scalarID(t) || value == nil {
			continue
		}
		k := other.identities[parent][value]
		if chosen, ok := p.selected[parent][k]; ok && chosen.r != nil && side.entries[parent][k] == nil {
			return true
		}
	}
	return false
}

func (p *threeWayPlan) parentsSurvive(t string, v *snapshot, r row) bool {
	for col, value := range r {
		parent := foreignTable(col)
		if value == nil || col == scalarID(t) || parent == "" || parent == "Location" || parent == "IndependentMedia" || parent == "PlaylistItemAccuracy" || t == "Note" && parent == "UserMark" {
			continue
		}
		if _, ok := p.selected[parent][v.identities[parent][value]]; !ok {
			return false
		}
	}
	return true
}

func (p *threeWayPlan) recordDelete(t, k string, base row, missing []string, reason string, r *Report) {
	if base == nil {
		return
	}
	r.Deletions = append(r.Deletions, Deletion{Table: t, Key: k, DeletedIn: missing, Reason: reason, Resolution: "deleted"})
	r.DeletedCounts[t]++
}

func (p *threeWayPlan) decide(t, k string, base, a, b row, first, second *snapshot, opts MergeOptions, r *Report) (selection, error) {
	if base == nil && a == nil {
		return selection{second, b}, nil
	}
	if base == nil && b == nil {
		return selection{first, a}, nil
	}
	missing := []string{}
	if a == nil {
		missing = append(missing, first.s.name)
	}
	if b == nil {
		missing = append(missing, second.s.name)
	}
	if a == nil && b == nil {
		p.recordDelete(t, k, base, missing, "removed", r)
		return selection{}, nil
	}
	dependent := a == nil || b == nil
	var ca, cb, ancestor row
	if a != nil {
		ca = first.comparable(t, a, dependent)
	}
	if b != nil {
		cb = second.comparable(t, b, dependent)
	}
	if base != nil {
		ancestor = p.base.comparable(t, base, dependent)
	}
	if dependent && (reflect.DeepEqual(ca, ancestor) || reflect.DeepEqual(cb, ancestor)) {
		p.recordDelete(t, k, base, missing, "removed", r)
		return selection{}, nil
	}
	if !dependent {
		if reflect.DeepEqual(ca, cb) {
			return selection{first, a}, nil
		}
		if base != nil && reflect.DeepEqual(ca, ancestor) {
			return selection{second, b}, nil
		}
		if base != nil && reflect.DeepEqual(cb, ancestor) {
			return selection{first, a}, nil
		}
	}
	conflict := Conflict{Table: t, Key: k, Winner: first.s.name, Other: second.s.name, Resolution: "preferred", Kind: "edit_edit"}
	if dependent {
		conflict.Kind = "delete_edit"
	}
	fields := map[string]bool{}
	for f, v := range ca {
		if !reflect.DeepEqual(v, cb[f]) {
			fields[f] = true
		}
	}
	for f, v := range cb {
		if !reflect.DeepEqual(v, ca[f]) {
			fields[f] = true
		}
	}
	for f := range fields {
		conflict.Fields = append(conflict.Fields, f)
	}
	sort.Strings(conflict.Fields)
	chosen := selection{first, a}
	if opts.Resolve != nil {
		x, y := resolverVersion(a, first), resolverVersion(b, second)
		resolved, err := opts.Resolve(conflict, x, y)
		if err != nil {
			return selection{}, err
		}
		if resolved != nil {
			origin, _ := resolved["_source"].(string)
			switch origin {
			case first.s.name:
				chosen.snap = first
			case second.s.name:
				chosen.snap = second
			default:
				return selection{}, errors.New("resolver must preserve the version source")
			}
			chosen.r = copyRow(resolved)
			delete(chosen.r, "_source")
			if deleted, _ := chosen.r["_deleted"].(bool); deleted {
				chosen.r = nil
			} else {
				delete(chosen.r, "_deleted")
			}
			conflict.Resolution = "interactive"
		}
	}
	r.Conflicts = append(r.Conflicts, conflict)
	if dependent {
		if chosen.r == nil {
			p.recordDelete(t, k, base, missing, "delete_edit", r)
		} else {
			r.Deletions = append(r.Deletions, Deletion{Table: t, Key: k, DeletedIn: missing, Reason: "delete_edit", Resolution: "kept_edited"})
		}
	}
	return chosen, nil
}

func resolverVersion(r row, v *snapshot) row {
	if r == nil {
		return row{"_deleted": true, "_source": v.s.name}
	}
	out := copyRow(r)
	out["_source"] = v.s.name
	return out
}

func (p *threeWayPlan) bindAliases(t string) {
	if t == "Location" || t == "IndependentMedia" || t == "PlaylistItemAccuracy" {
		return
	}
	for k, chosen := range p.selected[t] {
		id := scalarID(t)
		if id == "" {
			continue
		}
		mapped, ok := chosen.snap.s.ids[t][chosen.r[id]]
		if !ok {
			continue
		}
		for _, v := range p.sides {
			if r, ok := v.entries[t][k]; ok {
				v.s.ids[t][r[id]] = mapped
			}
		}
	}
}

func (p *threeWayPlan) reportRangeDeletions(report *Report) {
	for markKey, baseMark := range p.base.entries["UserMark"] {
		chosen, exists := p.selected["UserMark"][markKey]
		kept := map[string]bool{}
		if exists {
			for _, r := range chosen.snap.s.rows["BlockRange"] {
				if r["UserMarkId"] == chosen.r["UserMarkId"] {
					kept[key(r, "BlockType", "Identifier", "StartToken", "EndToken")] = true
				}
			}
		}
		for _, r := range p.base.s.rows["BlockRange"] {
			if r["UserMarkId"] != baseMark["UserMarkId"] {
				continue
			}
			rangeKey := key(r, "BlockType", "Identifier", "StartToken", "EndToken")
			if kept[rangeKey] {
				continue
			}
			reason := "range_changed"
			if !exists {
				reason = "parent_deleted"
			}
			report.Deletions = append(report.Deletions, Deletion{Table: "BlockRange", Key: markKey + ":" + rangeKey, Reason: reason, Resolution: "deleted"})
			report.DeletedCounts["BlockRange"]++
		}
	}
}
