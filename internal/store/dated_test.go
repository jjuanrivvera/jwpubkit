package store

import (
	"testing"

	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// A yearly volume carries the year in its own symbol — es26, mwb26 — and the
// family is recorded separately. A caller knows the family, so a lookup by family
// has to find the volume: matching only the symbol made a synced publication
// invisible while the library held 365 dated rows for it.
func TestDatedDocsFindAVolumeByItsFamily(t *testing.T) {
	s, err := Open(t.TempDir(), "S")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	pub := testutil.Pub{
		Symbol: "es26", Undated: "es", Year: 2026, Title: "Volumen anual de prueba",
		Docs: []testutil.Doc{{ID: 1, MepsID: 2026001, Class: 4, Title: "Septiembre",
			HTML: `<p id="p1" data-pid="1">Texto inventado del día.</p>`}},
		Dated: [][4]any{{1, 20260930, 20260930, "p/S:2026001/1-1"}},
	}
	if _, err := s.IndexLocal(testutil.Build(t, t.TempDir(), pub), "es26", "", "S"); err != nil {
		t.Fatal(err)
	}

	// By the family, which is what a caller has.
	byFamily, err := s.DatedDocs("es", 20260930)
	if err != nil {
		t.Fatal(err)
	}
	if len(byFamily) != 1 {
		t.Errorf("DatedDocs(\"es\") found %d documents; the volume is synced as es26", len(byFamily))
	}
	// And by the volume's own symbol, which still has to work.
	bySymbol, err := s.DatedDocs("es26", 20260930)
	if err != nil {
		t.Fatal(err)
	}
	if len(bySymbol) != 1 {
		t.Errorf("DatedDocs(\"es26\") found %d documents", len(bySymbol))
	}
	// A date the volume does not cover finds nothing rather than the nearest thing.
	if out, _ := s.DatedDocs("es", 20261231); len(out) != 0 {
		t.Errorf("a date outside the volume matched %d documents", len(out))
	}
}
