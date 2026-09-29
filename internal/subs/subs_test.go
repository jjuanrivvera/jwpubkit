package subs

import (
	"testing"
	"time"
)

const sample = `WEBVTT

00:00:01.730 --> 00:00:03.607 line:90% position:50% align:center
¿Se han preguntado
alguna vez

00:00:03.607 --> 00:00:06.485 line:90% position:50% align:center
antes de ir a <i>dormir</i>?

00:00:09.405 --> 00:00:12.325
No me refiero a eso &amp; nada más.

01:02:03.000 --> 01:02:04,500
Final.
`

func TestParseVTT(t *testing.T) {
	cues, err := ParseVTT(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 4 {
		t.Fatalf("%d cues", len(cues))
	}
	if cues[0].Start != 1730*time.Millisecond || cues[0].Text != "¿Se han preguntado\nalguna vez" || cues[0].From != "0:02" {
		t.Errorf("cue 0 %+v", cues[0])
	}
	if cues[1].Text != "antes de ir a dormir?" {
		t.Errorf("tags not stripped: %q", cues[1].Text)
	}
	if cues[2].Text != "No me refiero a eso & nada más." {
		t.Errorf("entities: %q", cues[2].Text)
	}
	if cues[3].Start != time.Hour+2*time.Minute+3*time.Second || cues[3].From != "1:02:03" {
		t.Errorf("hours: %+v", cues[3])
	}
}

func TestTranscript(t *testing.T) {
	cues, _ := ParseVTT(sample)
	got := Transcript(cues)
	want := "¿Se han preguntado alguna vez antes de ir a dormir?\n\nNo me refiero a eso & nada más.\n\nFinal."
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if _, err := ParseVTT("WEBVTT\n\n"); err == nil {
		t.Error("empty file must fail")
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"pub-jwb-125_4_VIDEO":     "pub-jwb-125_4_VIDEO",
		"pub-jwb-125_4":           "pub-jwb-125_4_VIDEO",
		"docid-702017141_1_VIDEO": "docid-702017141_1_VIDEO",
		"https://www.jw.org/finder?lank=pub-jwb-125_4_VIDEO&wtlocale=S": "pub-jwb-125_4_VIDEO",
		"webpubvid://?pub=jwbcov21&track=11&langwritten=S":              "pub-jwbcov21_11_VIDEO",
		"webpubvid://?pub=jwbai&issue=201507&track=1":                   "pub-jwbai_201507_1_VIDEO",
		"webpubvid://?docid=502016177&track=1":                          "docid-502016177_1_VIDEO",
		"jwb-125:4":                                                     "pub-jwb-125_4_VIDEO",
		"jwbai:201507:1":                                                "pub-jwbai_201507_1_VIDEO",
	}
	for in, want := range cases {
		got, err := NormalizeKey(in)
		if err != nil || got != want {
			t.Errorf("NormalizeKey(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "hola", "https://www.jw.org/es/"} {
		if _, err := NormalizeKey(bad); err == nil {
			t.Errorf("NormalizeKey(%q) should fail", bad)
		}
	}
}
