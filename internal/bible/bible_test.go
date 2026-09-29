package bible

import (
	"reflect"
	"testing"
)

func TestVersificationMatchesNwtsty(t *testing.T) {
	// nwtsty_S.jwpub (2026) has 31194 BibleVerse rows: 31078 verses plus 116
	// Psalm superscriptions.
	if TotalVerseIDs != 31194 {
		t.Fatalf("TotalVerseIDs = %d, want 31194", TotalVerseIDs)
	}
	cases := []struct {
		b, c, v, id int
	}{
		{1, 1, 1, 0},       // Génesis 1:1
		{1, 1, 2, 1},       //
		{24, 38, 1, 20012}, // Jeremías 38:1 (BibleChapter.FirstVerseId)
		{24, 38, 6, 20017}, // the verse used in the crypto test
		{24, 38, 28, 20039},
		{24, 39, 1, 20040},
		{19, 3, 0, 13958}, // Salmo 3 superscription
		{19, 3, 1, 13959},
		{43, 3, 16, 26240}, // Juan 3:16
		{66, 22, 21, 31193},
	}
	for _, c := range cases {
		id, ok := VerseID(c.b, c.c, c.v)
		if !ok || id != c.id {
			t.Errorf("VerseID(%d,%d,%d) = %d,%v want %d", c.b, c.c, c.v, id, ok, c.id)
		}
		b, ch, v, ok := Locate(c.id)
		if !ok || b != c.b || ch != c.c || v != c.v {
			t.Errorf("Locate(%d) = %d %d:%d, want %d %d:%d", c.id, b, ch, v, c.b, c.c, c.v)
		}
	}
	for _, bad := range [][3]int{{24, 38, 29}, {24, 53, 1}, {1, 1, 0}, {67, 1, 1}} {
		if _, ok := VerseID(bad[0], bad[1], bad[2]); ok {
			t.Errorf("VerseID%v should fail", bad)
		}
	}
}

func TestLocateRoundTripAll(t *testing.T) {
	for id := 0; id < TotalVerseIDs; id++ {
		b, c, v, ok := Locate(id)
		if !ok {
			t.Fatalf("Locate(%d) failed", id)
		}
		back, ok := VerseID(b, c, v)
		if !ok || back != id {
			t.Fatalf("round trip %d -> %d %d:%d -> %d", id, b, c, v, back)
		}
	}
}

func TestLookupBook(t *testing.T) {
	cases := map[string]int{
		"Jer": 24, "Jer.": 24, "jeremias": 24, "Jeremías": 24, "JEREMÍAS": 24,
		"1 Cor.": 46, "1Co": 46, "1 corintios": 46, "1cor": 46,
		"Sal.": 19, "Sl": 19, "Salmos": 19, "salmo": 19,
		"Hech.": 44, "Hch": 44, "hechos": 44,
		"Juan": 43, "Jn": 43, "1 Juan": 62, "1Jn": 62, "3 Juan": 64,
		"Apoc.": 66, "Ap": 66, "Apocalipsis": 66, "Revelación": 66,
		"Cant.": 22, "El Cantar de los Cantares": 22, "Cantar de los Cantares": 22,
		"Gé": 1, "Gén.": 1, "Éx": 2, "éxodo": 2, "Snt": 59, "Sant.": 59,
		"Flp": 50, "Filip.": 50, "Flm": 57, "Jud": 65, "Jue": 7, "Ezeq.": 26,
	}
	for in, want := range cases {
		b, ok := LookupBook(in)
		if !ok || b.Num != want {
			t.Errorf("LookupBook(%q) = %d,%v want %d", in, b.Num, ok, want)
		}
	}
	for _, bad := range []string{"", "Jerem", "Mormón", "xyz"} {
		if _, ok := LookupBook(bad); ok {
			t.Errorf("LookupBook(%q) should fail", bad)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want []Range
		str  string
	}{
		{"Jer 38:6", []Range{{24, 38, 6, 38, 6}}, "Jer 38:6"},
		{"Jer 38:1-13", []Range{{24, 38, 1, 38, 13}}, "Jer 38:1-13"},
		{"Jeremías 38:1-13", []Range{{24, 38, 1, 38, 13}}, "Jer 38:1-13"},
		{"Jer. 38:1–13", []Range{{24, 38, 1, 38, 13}}, "Jer 38:1-13"},
		{"Jeremías 38, 39", []Range{{24, 38, 1, 38, 28}, {24, 39, 1, 39, 18}}, "Jer 38; 39"},
		{"Jer 38", []Range{{24, 38, 1, 38, 28}}, "Jer 38"},
		{"Jer 38-39", []Range{{24, 38, 1, 39, 18}}, "Jer 38-39"},
		{"Jer 38:28–39:2", []Range{{24, 38, 28, 39, 2}}, "Jer 38:28-39:2"},
		{"Jer 39:6, 7", []Range{{24, 39, 6, 39, 6}, {24, 39, 7, 39, 7}}, "Jer 39:6; 39:7"},
		{"Jer 38:6; 39:1, 4-6", []Range{{24, 38, 6, 38, 6}, {24, 39, 1, 39, 1}, {24, 39, 4, 39, 6}}, "Jer 38:6; 39:1; 39:4-6"},
		{"1 Cor. 13:4-7", []Range{{46, 13, 4, 13, 7}}, "1Co 13:4-7"},
		{"1Co 13:4, 7", []Range{{46, 13, 4, 13, 4}, {46, 13, 7, 13, 7}}, "1Co 13:4; 13:7"},
		{"3 Juan 3, 4", []Range{{64, 1, 3, 1, 3}, {64, 1, 4, 1, 4}}, "3Jn 3; 4"},
		{"Jud 3-5", []Range{{65, 1, 3, 1, 5}}, "Jud 3-5"},
		{"Sal 3", []Range{{19, 3, 0, 3, 8}}, "Sl 3"},
		{"Sal. 25:12-15", []Range{{19, 25, 12, 25, 15}}, "Sl 25:12-15"},
		{"Juan 17:3; Mateo 24:14", []Range{{43, 17, 3, 17, 3}, {40, 24, 14, 24, 14}}, "Jn 17:3; Mt 24:14"},
		{"El Cantar de los Cantares 8:6", []Range{{22, 8, 6, 8, 6}}, "Can 8:6"},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
		if s := FormatList(got); s != c.str {
			t.Errorf("format(%q) = %q, want %q", c.in, s, c.str)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "Jer", "Jer 53:1", "Jer 38:29", "Libro 1:1", "38:6", "Jer 38:10-5"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestParseLink(t *testing.T) {
	r, ok := ParseLink("jwpub://b/NWTR/24:38:1-24:38:13")
	if !ok || r != (Range{24, 38, 1, 38, 13}) {
		t.Fatalf("got %+v %v", r, ok)
	}
	r, ok = ParseLink("jwpub://b/NWTR/43:17:3")
	if !ok || r != (Range{43, 17, 3, 17, 3}) {
		t.Fatalf("got %+v %v", r, ok)
	}
	if _, ok := ParseLink("jwpub://p/S:2013043/22-22"); ok {
		t.Fatal("publication link parsed as Bible link")
	}
	if r.Long() != "Juan 17:3" {
		t.Fatalf("Long() = %q", r.Long())
	}
}

func TestFormatList(t *testing.T) {
	rs := []Range{{24, 38, 7, 38, 9}, {24, 39, 15, 39, 18}, {43, 17, 3, 17, 3}}
	if got := FormatList(rs); got != "Jer 38:7-9; 39:15-18; Jn 17:3" {
		t.Fatalf("FormatList = %q", got)
	}
}
