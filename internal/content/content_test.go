package content

import (
	"strings"
	"testing"
)

// Synthetic fragments that reproduce the markup of real JWPUB documents
// (class names, data-pid, parNum, gen-field, figures) with made-up text.
const studyArticle = `<header>
<p class="contextTtl" id="p1" data-pid="1"><strong>5-11 DE ENERO DE 2026</strong></p>
<p id="p2" data-pid="2" class="pubRefs"><a class="xt" href="jwpub://p/S:1102016801/"><strong>CANCIÓN 1</strong></a> Título de canción</p>
<h1 id="p3" data-pid="3"><strong>Un artículo de prueba</strong></h1>
</header>
<p id="p4" data-pid="4" class="themeScrp"><em>“Texto temático”</em> (<a href="jwpub://b/NWTR/43:17:3-43:17:3" class="b">JUAN 17:3</a>).</p>
<div class="bodyTxt">
<p id="p20" data-pid="20" class="qu"> <strong>1.</strong> ¿Primera pregunta?</p>
<div class="gen-field" id="p21" data-pid="21"><label>Respuesta</label><textarea></textarea></div>
<p id="p5" data-pid="5" data-rel-pid="[20]"><span class="parNum" data-pnum="1"></span>PRIMER párrafo con nota<span data-fnid="1" class="fn">a<span class="tt fn"></span></span> y <strong>(lee</strong> <a href="jwpub://b/NWTR/54:2:3-54:2:4" class="b"><strong>1 Timoteo 2:3, 4</strong></a><strong>).</strong></p>
<h2 id="p6" data-pid="6"><strong>UN SUBTÍTULO</strong></h2>
<p id="p22" data-pid="22" class="qu"><strong>2,` + " " + `3.</strong> a) ¿Pregunta doble? (<a href="jwpub://b/NWTR/59:5:11-59:5:11" class="b">Santiago 5:11</a>).</p>
<div class="gen-field" id="p23" data-pid="23"></div>
<p id="p7" data-pid="7" data-rel-pid="[22]"><span class="parNum" data-pnum="2"><strong><sup>2</sup></strong></span> Segundo párrafo (<a class="xt" href="jwpub://p/S:2013043/22-22"><em>w13</em> 15/1 9 párr. 12</a>).</p>
<div id="f1"><figure><img src="jwpub-media://2026999_univ_cnt_1.jpg" alt="Descripción de la imagen." width="1200" height="675" /><p id="p30" data-pid="30" class="imgCredit">Cortesía de alguien</p><figcaption class="figcaption"><p id="p31" data-pid="31">Pie de la imagen.</p></figcaption></figure></div>
<p id="p8" data-pid="8"><span class="parNum" data-pnum="3"><strong><sup>3</sup></strong></span> Tercero con <span class="pageNum" data-no="9"></span>página.</p>
</div>
<div class="blockTeach"><aside><div id="p9" data-pid="9" class="boxTtl"><h2><strong>¿QUÉ RESPONDERÍAS?</strong></h2></div>
<div class="boxContent"><ul><li><p id="p10" data-pid="10">  ¿Pregunta de repaso?</p><div class="gen-field" id="p11" data-pid="11"></div></li></ul></div></aside></div>
<p id="p12" data-pid="12"><a href="https://www.jw.org/finder?lank=pub-jwb-125_4_VIDEO&amp;wtlocale=S" data-video="webpubvid://?pub=jwb-125&amp;track=4&amp;langwritten=S"><strong>Ponga el VIDEO</strong></a>.</p>
<video data-video="webpubvid://?pub=thv&amp;track=12&amp;style=chromeless"></video>
<div class="groupFootnote"><div id="footnote1" data-fnid="1" class="fn-ref"><p id="p13" data-pid="13"><a href="#footnotesource1" class="fn-symbol">a</a> Texto de la nota.</p></div></div>`

func parse(t *testing.T, src string) *Doc {
	t.Helper()
	d, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func block(d *Doc, pid int) *Block {
	for _, b := range d.Blocks {
		if b.PID == pid {
			return b
		}
	}
	return nil
}

func TestParseStudyArticle(t *testing.T) {
	d := parse(t, studyArticle)
	cases := []struct {
		pid      int
		kind     string
		num      int
		label    string
		text     string
		question bool
	}{
		{1, KindContext, 0, "", "5-11 DE ENERO DE 2026", false},
		{2, KindMeta, 0, "", "CANCIÓN 1 Título de canción", false},
		{3, KindHeading, 0, "", "Un artículo de prueba", false},
		{4, KindTheme, 0, "", "“Texto temático” (JUAN 17:3).", false},
		{20, KindQuestion, 1, "1", "¿Primera pregunta?", true},
		{5, KindPara, 1, "", "PRIMER párrafo con nota y (lee 1 Timoteo 2:3, 4).", false},
		{22, KindQuestion, 2, "2, 3", "a) ¿Pregunta doble? (Santiago 5:11).", true},
		{7, KindPara, 2, "", "Segundo párrafo (w13 15/1 9 párr. 12).", false},
		{31, KindCaption, 0, "", "Pie de la imagen.", false},
		{8, KindPara, 3, "", "Tercero con página.", false},
		{9, KindHeading, 0, "", "¿QUÉ RESPONDERÍAS?", false},
		{10, KindItem, 0, "", "¿Pregunta de repaso?", true},
		{13, KindFootnote, 0, "", "Texto de la nota.", false},
	}
	for _, c := range cases {
		b := block(d, c.pid)
		if b == nil {
			t.Errorf("pid %d missing", c.pid)
			continue
		}
		if b.Kind != c.kind || b.Num != c.num || b.NumLabel != c.label || b.Text() != c.text || b.IsQuestion() != c.question {
			t.Errorf("pid %d = {%s %d %q %q q=%v}, want {%s %d %q %q q=%v}", c.pid,
				b.Kind, b.Num, b.NumLabel, b.Text(), b.IsQuestion(), c.kind, c.num, c.label, c.text, c.question)
		}
	}
	if b := block(d, 5); b.RelPID != 20 {
		t.Errorf("RelPID = %d, want 20", b.RelPID)
	}
	if b := block(d, 9); !b.InBox || b.Level != 2 {
		t.Errorf("box heading: InBox=%v Level=%d", b.InBox, b.Level)
	}
	if b := block(d, 13); b.FnLabel != "a" {
		t.Errorf("footnote label %q", b.FnLabel)
	}
	if block(d, 21) != nil || block(d, 11) != nil {
		t.Error("answer boxes must not become blocks")
	}
}

func TestParseLinksImagesVideos(t *testing.T) {
	d := parse(t, studyArticle)
	b := block(d, 5)
	refs := b.BibleRefs()
	if len(refs) != 1 || refs[0].String() != "1Ti 2:3-4" {
		t.Errorf("bible refs %v", refs)
	}
	pl := block(d, 7).PubLinks()
	if len(pl) != 1 || pl[0].DocID != 2013043 || pl[0].Pars != "22-22" || pl[0].First != 22 || pl[0].Text != "w13 15/1 9 párr. 12" {
		t.Errorf("pub link %+v", pl)
	}
	if len(d.Images) != 1 {
		t.Fatalf("images %d", len(d.Images))
	}
	img := d.Images[0]
	if img.File != "2026999_univ_cnt_1.jpg" || img.Width != 1200 || img.Caption != "Pie de la imagen." || img.Credit != "Cortesía de alguien" || img.PID != 7 {
		t.Errorf("image %+v", img)
	}
	var keys []string
	for _, v := range d.Videos {
		keys = append(keys, v.Key)
	}
	if strings.Join(keys, ",") != "pub-jwb-125_4_VIDEO,pub-thv_12_VIDEO" {
		t.Errorf("videos %v", keys)
	}
}

func TestSubentryNumbering(t *testing.T) {
	// An Insight entry with numbered senses: citations count per sense.
	src := `<h1 id="p1" data-pid="1"><strong>NOMBRE</strong></h1>
<p id="p2" data-pid="2" class="sn"><span class="parNum" data-pnum="1"></span>(Significado).</p>
<p id="p3" data-pid="3" class="sb"><strong>1.</strong> Primer sentido.</p>
<p id="p4" data-pid="4" class="sb"><strong>2.</strong> Segundo sentido.</p>
<p id="p5" data-pid="5" class="sb">Sigue el segundo.</p>
<p id="p6" data-pid="6" class="sc">Pie de un grabado.</p>
<p id="p7" data-pid="7" class="sb">Y otro más.</p>`
	d := parse(t, src)
	want := map[int][2]int{3: {1, 1}, 4: {2, 1}, 5: {2, 2}, 7: {2, 3}, 2: {0, 0}, 6: {0, 0}}
	for pid, w := range want {
		b := block(d, pid)
		if b.Sub != w[0] || b.Num != w[1] {
			t.Errorf("pid %d: núm %d párr %d, want %v", pid, b.Sub, b.Num, w)
		}
	}
	// Without senses, parNum is the number publications cite (Egipto párr. 28 = pid 30).
	d = parse(t, `<p id="p30" data-pid="30" class="sb"><span class="parNum" data-pnum="28"></span>Texto.</p>`)
	if b := block(d, 30); b.Num != 28 || b.Sub != 0 {
		t.Errorf("parNum: %+v", b)
	}
}

func TestMarkdown(t *testing.T) {
	md := parse(t, studyArticle).Markdown(RenderOptions{})
	for _, want := range []string{
		"*5-11 DE ENERO DE 2026*",
		"# Un artículo de prueba",
		"> **1.** ¿Primera pregunta?",
		"**1** PRIMER párrafo con nota[^a] y **(lee 1 Timoteo 2:3, 4).**",
		"> **2, 3.** a) ¿Pregunta doble? (Santiago 5:11).",
		"[*w13* 15/1 9 párr. 12](https://wol.jw.org/es/wol/d/r4/lp-s/2013043#p22)",
		"![Descripción de la imagen.](2026999_univ_cnt_1.jpg)\n*Pie de la imagen.*",
		"> ## ¿QUÉ RESPONDERÍAS?",
		"> - ¿Pregunta de repaso?",
		"[^a]: Texto de la nota.",
		"[**Ponga el VIDEO**](https://www.jw.org/finder?lank=pub-jwb-125_4_VIDEO&wtlocale=S)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q\n---\n%s", want, md)
		}
	}
	if strings.Contains(md, "Respuesta") {
		t.Error("answer box labels leaked into the markdown")
	}
}

func TestPlainText(t *testing.T) {
	txt := parse(t, studyArticle).PlainText()
	for _, want := range []string{
		"[H1] Un artículo de prueba",
		"[PREGUNTA 1] ¿Primera pregunta?",
		"1 PRIMER párrafo con nota(a) y (lee 1 Timoteo 2:3, 4).",
		"[IMAGEN alt=Descripción de la imagen. | archivo=2026999_univ_cnt_1.jpg | pie=Pie de la imagen.]",
		"[TEXTO TEMÁTICO]",
		"[NOTA a] Texto de la nota.",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q\n---\n%s", want, txt)
		}
	}
}

func TestNoBreakSpaceSurvives(t *testing.T) {
	d := parse(t, `<p id="p1" data-pid="1">no`+" "+`había   agua</p>`)
	if got := block(d, 1).Text(); got != "no había agua" {
		t.Errorf("got %q", got)
	}
}

func TestVideoKey(t *testing.T) {
	cases := map[[4]string]string{
		{"jwb-125", "", "4", ""}:      "pub-jwb-125_4_VIDEO",
		{"jwbai", "201507", "1", ""}:  "pub-jwbai_201507_1_VIDEO",
		{"mwbv", "20260900", "3", ""}: "pub-mwbv_202609_3_VIDEO",
		{"", "", "1", "702017141"}:    "docid-702017141_1_VIDEO",
		{"", "", "", ""}:              "",
	}
	for in, want := range cases {
		track := 0
		if in[2] != "" {
			track = int(in[2][0] - '0')
		}
		docid := 0
		if in[3] != "" {
			docid = 702017141
		}
		if got := VideoKey(in[0], in[1], track, docid); got != want {
			t.Errorf("VideoKey%v = %q, want %q", in, got, want)
		}
	}
}

func TestChapterMarks(t *testing.T) {
	src := `<p id="p1" data-pid="1"><span id="v24-38-2-1" class="v"><span class="vl">2 <span class="tt vl"></span></span>por la espada y la peste.<span data-fnid="331" class="fn pr">a<span class="tt fn"></span></span><span data-mid="1191" class="m pr">c<span class="tt m"></span></span> Pero el que se rinda<span data-fnid="332" class="fn">b<span class="tt fn"></span></span></span></p>`
	fns, mids := ChapterMarks(src)
	if m := fns[331]; m.Verse != 2 || m.Letter != "a" || m.Anchor != "peste" {
		t.Errorf("fn 331 = %+v", m)
	}
	if m := fns[332]; m.Letter != "b" || m.Anchor != "rinda" {
		t.Errorf("fn 332 = %+v", m)
	}
	if m := mids[1191]; m.Book != 24 || m.Chapter != 38 || m.Letter != "c" || m.Anchor != "peste" {
		t.Errorf("mid 1191 = %+v", m)
	}
}
