package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func descendant(t *testing.T, base, name string, mutate func(*sql.DB)) string {
	t.Helper()
	a, err := Open(t.Context(), base)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	// These disposable databases need no crash durability; archive validation still runs.
	execTest(t, a.DB, "PRAGMA journal_mode=MEMORY; PRAGMA synchronous=OFF")
	if mutate != nil {
		mutate(a.DB)
	}
	out := filepath.Join(t.TempDir(), name+".jwlibrary")
	if err = a.Save(t.Context(), out, name); err != nil {
		t.Fatal(err)
	}
	return out
}

func ancestorFixture(t *testing.T) string {
	return fixture(t, "ancestor", "2026-01-01T00:00:00Z", func(db *sql.DB) {
		seedRows(t, db, 0, "Invented original")
		execTest(t, db, `UPDATE Note SET Created='2026-01-01T00:00:00Z',LastModified='2026-01-01T00:00:00Z'; INSERT INTO PlaylistItemAccuracy VALUES(0,'Invented precise'); INSERT INTO PlaylistItem VALUES(90,'Invented item',0,10,0,0,NULL); INSERT INTO IndependentMedia VALUES(80,'invented.bin','media.bin','application/octet-stream','synthetic-hash'); INSERT INTO PlaylistItemIndependentMediaMap VALUES(90,80,10); INSERT INTO PlaylistItemLocationMap VALUES(90,1,1,10); INSERT INTO PlaylistItemMarker VALUES(100,90,'Invented marker',0,10,0); INSERT INTO PlaylistItemMarkerBibleVerseMap VALUES(100,42); INSERT INTO PlaylistItemMarkerParagraphMap VALUES(100,123456,1,0); INSERT INTO Tag VALUES(111,2,'Invented playlist'); INSERT INTO TagMap(TagMapId,PlaylistItemId,TagId,Position) VALUES(112,90,111,0)`)
	}, map[string][]byte{"media.bin": []byte("Invented bytes")})
}

func TestThreeWayDeletions(t *testing.T) {
	base := ancestorFixture(t)
	cases := []struct {
		table, remove, edit string
		want                int
	}{
		{"Note", "DELETE FROM TagMap WHERE NoteId=4; DELETE FROM Note", "UPDATE Note SET Content='Invented revision'", 0},
		{"UserMark", "UPDATE Note SET UserMarkId=NULL; DELETE FROM BlockRange; DELETE FROM UserMark", "UPDATE BlockRange SET EndToken=8", 0},
		{"InputField", "DELETE FROM InputField", "UPDATE InputField SET Value='Invented revision'", 0},
		{"Bookmark", "DELETE FROM Bookmark", "UPDATE Bookmark SET Title='Invented revision'", 0},
		{"Tag", "DELETE FROM TagMap WHERE TagId=5; DELETE FROM Tag WHERE TagId=5", "UPDATE Tag SET Name='Invented renamed' WHERE TagId=5", 1},
		{"TagMap", "DELETE FROM TagMap WHERE NoteId=4", "UPDATE TagMap SET Position=3 WHERE NoteId=4", 1},
		{"PlaylistItem", "DELETE FROM TagMap WHERE PlaylistItemId=90; DELETE FROM PlaylistItemMarkerBibleVerseMap; DELETE FROM PlaylistItemMarkerParagraphMap; DELETE FROM PlaylistItemMarker; DELETE FROM PlaylistItemLocationMap; DELETE FROM PlaylistItemIndependentMediaMap; DELETE FROM PlaylistItem", "UPDATE PlaylistItem SET Label='Invented revision'", 0},
		{"PlaylistItemMarker", "DELETE FROM PlaylistItemMarkerBibleVerseMap; DELETE FROM PlaylistItemMarkerParagraphMap; DELETE FROM PlaylistItemMarker", "UPDATE PlaylistItemMarker SET DurationTicks=20", 0},
		{"PlaylistItemIndependentMediaMap", "DELETE FROM PlaylistItemIndependentMediaMap", "UPDATE PlaylistItemIndependentMediaMap SET DurationTicks=20", 0},
		{"PlaylistItemLocationMap", "DELETE FROM PlaylistItemLocationMap", "UPDATE PlaylistItemLocationMap SET BaseDurationTicks=20", 0},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			removed := descendant(t, base, "removed", func(db *sql.DB) { execTest(t, db, tc.remove) })
			same := descendant(t, base, "unchanged", nil)
			edited := descendant(t, base, "edited", func(db *sql.DB) { execTest(t, db, tc.edit) })
			report, err := Merge(t.Context(), []string{same, removed}, "", MergeOptions{Base: base, DryRun: true, Prefer: same})
			if err != nil {
				t.Fatal(err)
			}
			if report.After[tc.table] != tc.want || report.DeletedCounts[tc.table] != 1 {
				t.Fatalf("simple deletion: %+v", report)
			}
			for _, keep := range []bool{false, true} {
				prefer := removed
				want := tc.want
				if keep {
					prefer = edited
					want++
				}
				out := filepath.Join(t.TempDir(), "result.jwlibrary")
				r, e := Merge(t.Context(), []string{removed, edited}, out, MergeOptions{Base: base, Prefer: prefer})
				if e != nil {
					t.Fatal(e)
				}
				if r.After[tc.table] != want {
					t.Fatalf("keep=%v: %+v", keep, r)
				}
				found := false
				for _, c := range r.Conflicts {
					if c.Table == tc.table && c.Kind == "delete_edit" {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing deletion conflict: %+v", r)
				}
				a, e := Open(t.Context(), out)
				if e != nil {
					t.Fatal(e)
				}
				a.Close()
				if tc.table == "PlaylistItem" && keep && r.After["PlaylistItemMarker"] != 1 {
					t.Fatalf("restored item lost its children: %+v", r.After)
				}
				if tc.table == "UserMark" && !keep && r.After["Note"] != 1 {
					t.Fatal("mark deletion removed a surviving note")
				}
			}
			r, e := Merge(t.Context(), []string{removed, same}, "", MergeOptions{DryRun: true})
			if e != nil {
				t.Fatal(e)
			}
			if r.After[tc.table] != tc.want+1 {
				t.Fatal("union without ancestor changed")
			}
		})
	}
}

func TestThreeWayEditsAdditionsAndRecreation(t *testing.T) {
	base := ancestorFixture(t)
	unchanged := descendant(t, base, "unchanged", nil)
	changed := descendant(t, base, "changed", func(db *sql.DB) {
		execTest(t, db, `UPDATE Note SET Content='Invented changed'; INSERT INTO Note(NoteId,Guid,Content) VALUES(20,'invented-new-guid','Invented addition')`)
	})
	out := filepath.Join(t.TempDir(), "out.jwlibrary")
	r, err := Merge(t.Context(), []string{unchanged, changed}, out, MergeOptions{Base: base, Prefer: unchanged})
	if err != nil {
		t.Fatal(err)
	}
	if r.After["Note"] != 2 {
		t.Fatal(r.After)
	}
	a, err := Open(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	err = a.DB.QueryRowContext(t.Context(), "SELECT Content FROM Note WHERE Guid='shared-note'").Scan(&text)
	a.Close()
	if err != nil || text != "Invented changed" {
		t.Fatalf("one-sided edit lost: %s, %v", text, err)
	}
	removed := descendant(t, base, "removed", func(db *sql.DB) { execTest(t, db, `DELETE FROM TagMap WHERE NoteId=4; DELETE FROM Note`) })
	recreated := descendant(t, base, "recreated", func(db *sql.DB) {
		execTest(t, db, `DELETE FROM TagMap WHERE NoteId=4; DELETE FROM Note; INSERT INTO Note(NoteId,Guid,Created,Content) VALUES(400,'shared-note','2026-02-01T00:00:00Z','Invented recreation')`)
	})
	for _, choice := range []string{"a", "b", "skip"} {
		calls := 0
		r, err = Merge(t.Context(), []string{removed, recreated}, "", MergeOptions{Base: base, DryRun: true, Resolve: func(c Conflict, a, b map[string]any) (map[string]any, error) {
			calls++
			if choice == "a" {
				return a, nil
			}
			if choice == "b" {
				return b, nil
			}
			return nil, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if choice == "b" {
			want = 1
		}
		if calls == 0 || r.After["Note"] != want {
			t.Fatalf("recreation %s: %+v", choice, r)
		}
	}
	if _, err = Merge(t.Context(), []string{removed, changed, recreated}, "", MergeOptions{Base: base, DryRun: true}); err == nil {
		t.Fatal("ambiguous multiple descendants accepted")
	}
	if _, err = Merge(t.Context(), []string{removed, changed}, "", MergeOptions{Base: filepath.Join(t.TempDir(), "missing"), DryRun: true}); err == nil {
		t.Fatal("missing ancestor accepted")
	}
}

func TestThreeWayRemappedReferences(t *testing.T) {
	base := ancestorFixture(t)
	shifted := descendant(t, base, "shifted", func(db *sql.DB) {
		execTest(t, db, `PRAGMA foreign_keys=OFF; UPDATE Location SET LocationId=101; UPDATE UserMark SET UserMarkId=102,LocationId=101; UPDATE BlockRange SET UserMarkId=102; UPDATE Note SET NoteId=104,UserMarkId=102,LocationId=101; UPDATE Tag SET TagId=105 WHERE TagId=5; UPDATE TagMap SET TagId=105,NoteId=104 WHERE NoteId=4; UPDATE InputField SET LocationId=101,Value='Invented changed'; UPDATE Bookmark SET LocationId=101,PublicationLocationId=101; UPDATE PlaylistItemLocationMap SET LocationId=101; PRAGMA foreign_keys=ON;`)
	})
	other := descendant(t, base, "other", func(db *sql.DB) { execTest(t, db, `UPDATE Note SET Content='Invented changed'`) })
	out := filepath.Join(t.TempDir(), "out.jwlibrary")
	r, err := Merge(t.Context(), []string{shifted, other}, out, MergeOptions{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if r.After["Note"] != 1 || r.After["Tag"] != 2 || r.After["InputField"] != 1 {
		t.Fatal(r.After)
	}
	if _, err = os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
