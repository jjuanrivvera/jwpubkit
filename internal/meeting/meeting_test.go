package meeting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// weekHTML mirrors the markup of a Meeting Workbook week with invented text.
const weekHTML = `<header>
<h1 id="p1" data-pid="1">5-11 DE ENERO</h1>
<h2 id="p2" data-pid="2"><a href="jwpub://b/NWTR/1:1:1-1:1:31" class="b"><strong>GÉNESIS 1,</strong></a><a href="jwpub://b/NWTR/1:2:1-1:2:25" class="b"> <strong>2</strong></a></h2>
</header>
<div class="bodyTxt">
<h3 id="p3" data-pid="3"><a class="xt" href="jwpub://p/S:1102016801/"><strong>Canción 1</strong></a> <strong>y oración | Palabras de introducción</strong> <span>(1 min.)</span></h3>
<div><h2 id="p4" data-pid="4"><strong>TESOROS DE LA BIBLIA</strong></h2></div>
<div id="f1"><figure><img src="jwpub-media://900000001_univ_cnt_1.jpg" alt="Un paisaje." width="1200" height="675"/><figcaption><p id="p40" data-pid="40">Pie de prueba.</p></figcaption></figure></div>
<h3 id="p5" data-pid="5"><strong>1. Un título de prueba</strong></h3>
<div><p id="p6" data-pid="6">(10 mins.)</p></div>
<p id="p7" data-pid="7">Una idea (<a href="jwpub://b/NWTR/1:1:1-1:1:1" class="b">Gé 1:1</a>; <a class="xt" href="jwpub://p/S:900000010/5-5"><em>w99</em> 1/1 3 párr. 2</a>).</p>
<h3 id="p8" data-pid="8"><strong>2. Busquemos perlas escondidas</strong></h3>
<div><p id="p9" data-pid="9">(10 mins.)</p></div>
<ul><li><p id="p10" data-pid="10"><a href="jwpub://b/NWTR/1:1:26-1:1:27" class="b">Gé 1:26, 27</a>. ¿Qué pregunta de prueba?</p><div class="gen-field" id="p11" data-pid="11"><label>Respuesta</label><textarea></textarea></div></li></ul>
<h3 id="p12" data-pid="12"><strong>3. Lectura de la Biblia</strong></h3>
<div><p id="p13" data-pid="13">(4 mins.) <a href="jwpub://b/NWTR/1:2:1-1:2:9" class="b">Gé 2:1-9</a> (<a class="xt" href="jwpub://p/S:1102018445/"><em>th</em> lección 5</a>).</p></div>
<div><h2 id="p14" data-pid="14"><strong>NUESTRA VIDA CRISTIANA</strong></h2></div>
<h3 id="p15" data-pid="15"><a class="xt" href="jwpub://p/S:1102016802/"><strong>Canción 2</strong></a></h3>
<h3 id="p16" data-pid="16"><strong>4. <em>“Un video”</em></strong></h3>
<div><p id="p17" data-pid="17">(15 mins.) Análisis con el auditorio.</p></div>
<div id="f2"><figure><img src="jwpub-media://900000001_univ_cnt_2.jpg" alt="Otra escena." width="1200" height="675"/></figure></div>
<p id="p18" data-pid="18"><a href="https://www.jw.org/finder?lank=pub-jwb-999_1_VIDEO&amp;wtlocale=S" data-video="webpubvid://?pub=jwb-999&amp;track=1&amp;langwritten=S"><strong>Ponga el VIDEO</strong></a>. Luego pregunte:</p>
<ul><li><p id="p19" data-pid="19">¿Qué aprendemos?</p><div class="gen-field" id="p20" data-pid="20"></div></li></ul>
<h3 id="p21" data-pid="21"><strong>5. Estudio bíblico de la congregación</strong></h3>
<div><p id="p22" data-pid="22">(30 mins.) <a class="xt" href="jwpub://p/S:1102025901/"><em>wcg</em> cap. 1</a>.</p></div>
<h3 id="p23" data-pid="23"><strong>Palabras de conclusión</strong> <span>(3 mins.)</span> <strong>|</strong> <span><a class="xt" href="jwpub://p/S:1102016803/"><strong>Canción 3</strong></a></span> <strong>y oración</strong></h3>
</div>`

const chapterHTML = `<header><p class="contextTtl" id="p1" data-pid="1"><strong>1</strong> NOÉ</p>
<h1 id="p2" data-pid="2"><strong>Construyó el arca</strong></h1></header>
<div class="bodyTxt"><p id="p3" data-pid="3">Relato de prueba.</p>
<h3 id="p4" data-pid="4"><strong>Lea el relato bíblico</strong></h3>
<ul><li><p id="p5" data-pid="5"><a href="jwpub://b/NWTR/1:6:9-1:6:22" class="b"><strong>Génesis 6:9-22</strong></a></p></li></ul>
<h3 id="p6" data-pid="6"><strong>¿Qué diría?</strong></h3>
<p id="p7" data-pid="7"><strong>¿Qué valor mostró Noé?</strong></p><div class="gen-field" id="p8" data-pid="8"></div>
<h2 id="p9" data-pid="9"><strong>Para saber más</strong></h2>
<p id="p10" data-pid="10"><a href="https://www.jw.org/finder?lank=pub-jwbai_201507_1_VIDEO&amp;wtlocale=S" data-video="webpubvid://?pub=jwbai&amp;issue=201507&amp;track=1&amp;langwritten=S"><strong><em>Un video complementario</em> (4:58)</strong></a></p>
</div>`

func caption(loc, title string) string {
	return `<span class="eloc">` + loc + `</span> <span class="etitle">` + title + `</span>`
}

var workbook = testutil.Pub{
	Symbol: "mwb26", Undated: "mwb", Year: 2026, IssueTag: 20260100, Title: "Guía de prueba",
	Docs:  []testutil.Doc{{ID: 1, MepsID: 202026001, Class: 106, Title: "5-11 de enero", HTML: weekHTML}},
	Dated: [][4]any{{1, 20260105, 20260111, "p/S:202026001/1-23"}},
	Extracts: []testutil.Extract{
		{DocID: 1, ExtractID: 1, Link: "p/S:1102016801/", Caption: caption("sjj canción 1", "Primera canción"), HTML: "<p>letra</p>", RefDocID: 1102016801, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 3, Sort: 1},
		{DocID: 1, ExtractID: 2, Link: "p/S:900000010/5-5", Caption: caption("w99 1/1 pág. 3", "Un artículo viejo"), HTML: `<p id="p5" data-pid="5"><span class="parNum" data-pnum="2"></span>Texto citado.</p>`, RefDocID: 900000010, RefSymbol: "w99", RefUndated: "w", RefIssue: 19990101, BeginPID: 7, Sort: 2},
		{DocID: 1, ExtractID: 3, Link: "p/S:1102018445/", Caption: caption("th pág. 8", "Leer con exactitud"), HTML: "<p>lección</p>", RefDocID: 1102018445, RefSymbol: "th", RefUndated: "th", BeginPID: 13, Sort: 3},
		{DocID: 1, ExtractID: 4, Link: "p/S:1102016802/", Caption: caption("sjj canción 2", "Segunda canción"), HTML: "<p>letra</p>", RefDocID: 1102016802, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 15, Sort: 4},
		{DocID: 1, ExtractID: 5, Link: "p/S:1102025901/", Caption: caption("wcg págs. 4-9", "Construyó el arca"), HTML: chapterHTML, RefDocID: 1102025901, RefSymbol: "wcg", RefUndated: "wcg", BeginPID: 22, Sort: 5},
		{DocID: 1, ExtractID: 6, Link: "p/S:1102016803/", Caption: caption("sjj canción 3", "Tercera canción"), HTML: "<p>letra</p>", RefDocID: 1102016803, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 23, Sort: 6},
	},
	Media: []testutil.Media{
		{DocID: 1, ID: 1, File: "900000001_univ_cnt_1.jpg", Label: "Un paisaje.", Caption: "Pie de prueba.", BeginPID: 5, Width: 1200, Height: 675},
		{DocID: 1, ID: 2, File: "900000001_univ_cnt_2.jpg", Label: "Otra escena.", BeginPID: 16, Width: 1200, Height: 675},
	},
}

var watchtower = testutil.Pub{
	Symbol: "w25", Undated: "w", Year: 2025, IssueTag: 20251100, Title: "Atalaya de prueba",
	Docs: []testutil.Doc{
		{ID: 1, MepsID: 2025800, Class: 68, Title: "Índice", HTML: `<h3 id="p3" data-pid="3">Artículo de estudio del 5 al 11 de enero</h3>
<p id="p4" data-pid="4"><a href="jwpub://p/S:2025801/">2 Un artículo de estudio</a></p>`},
		{ID: 2, MepsID: 2025801, Class: 40, Title: "Un artículo de estudio", HTML: `<header><p class="contextTtl" id="p1" data-pid="1"><strong>5-11 DE ENERO DE 2026</strong></p>
<p id="p2" data-pid="2" class="pubRefs"><a class="xt" href="jwpub://p/S:1102016807/"><strong>CANCIÓN 7</strong></a> Canción de apertura</p>
<h1 id="p3" data-pid="3"><strong>Un artículo de estudio</strong></h1></header>
<p id="p4" data-pid="4" class="themeScrp"><em>“Tema”</em> (<a href="jwpub://b/NWTR/43:17:3-43:17:3" class="b">JUAN 17:3</a>).</p>
<p id="p5" data-pid="5" class="pubRefs"><strong>TEMA</strong></p><p id="p6" data-pid="6" class="pubRefs">De qué trata.</p>
<p id="p20" data-pid="20" class="qu"><strong>1.</strong> ¿Pregunta uno?</p><div class="gen-field" id="p21" data-pid="21"></div>
<p id="p7" data-pid="7"><span class="parNum" data-pnum="1"></span>Párrafo.</p>
<h2 id="p8" data-pid="8"><strong>SUBTÍTULO</strong></h2>
<p id="p22" data-pid="22" class="qu"><strong>2, 3.</strong> ¿Pregunta doble?</p><div class="gen-field" id="p23" data-pid="23"></div>
<div class="blockTeach"><aside><div id="p9" data-pid="9" class="boxTtl"><h2><strong>¿QUÉ RESPONDERÍAS?</strong></h2></div>
<div class="boxContent"><ul><li><p id="p10" data-pid="10">¿Repaso?</p><div class="gen-field" id="p11" data-pid="11"></div></li></ul></div></aside></div>
<p id="p12" data-pid="12" class="pubRefs"><a class="xt" href="jwpub://p/S:1102016808/"><strong>CANCIÓN 8</strong></a> Canción final</p>`},
	},
	Dated: [][4]any{{1, 20260105, 20260111, "p/S:2025800/3-4"}},
}

func TestBuildWeekSynthetic(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, p := range []struct {
		sym, iss string
		pub      testutil.Pub
	}{{"mwb", "202601", workbook}, {"w", "202511", watchtower}} {
		if _, err := st.IndexLocal(testutil.Build(t, t.TempDir(), p.pub), p.sym, p.iss, "S"); err != nil {
			t.Fatal(err)
		}
	}
	b := &Builder{Store: st}
	monday := Monday(time.Date(2026, 1, 8, 0, 0, 0, 0, time.Local)) // a Thursday
	if got := monday.Format("2006-01-02"); got != "2026-01-05" {
		t.Fatalf("Monday = %s", got)
	}
	dd, err := b.FindWorkbook(monday)
	if err != nil || dd == nil || dd.DocID != 202026001 {
		t.Fatalf("FindWorkbook = %+v %v", dd, err)
	}
	w := &Week{Monday: "2026-01-05"}
	if err := b.BuildWorkbook(w, dd.DocID, *dd); err != nil {
		t.Fatal(err)
	}

	if w.WeeklyReading == nil || w.WeeklyReading.Book != "Génesis" || len(w.WeeklyReading.Chapters) != 2 || w.WeeklyReading.Ref != "Gé 1; 2" {
		t.Errorf("weekly reading %+v", w.WeeklyReading)
	}
	if sr := w.StudentReading; sr == nil || sr.Ref != "Gé 2:1-9" || sr.Lesson == nil || sr.Lesson.DocID != 1102018445 || sr.Lesson.Title != "Leer con exactitud" {
		t.Errorf("student reading %+v", sr)
	}
	var songs []string
	for _, s := range w.Songs {
		songs = append(songs, s.When+":"+s.Title)
	}
	if strings.Join(songs, ",") != "inicio:Primera canción,medio:Segunda canción,final:Tercera canción" {
		t.Errorf("songs %v", songs)
	}

	parts := map[int]Part{}
	var titles []string
	for _, sec := range w.Sections {
		for _, p := range sec.Parts {
			parts[p.Number] = p
			titles = append(titles, p.Title)
		}
	}
	if strings.Join(titles, "|") != "Palabras de introducción|Un título de prueba|Busquemos perlas escondidas|Lectura de la Biblia|“Un video”|Estudio bíblico de la congregación|Palabras de conclusión" {
		t.Errorf("titles %q", titles)
	}
	if p := parts[1]; p.Minutes != 10 || len(p.Images) != 1 || p.Images[0].Caption != "Pie de prueba." {
		t.Errorf("part 1 %+v", p)
	}
	var ref Reference
	for _, r := range parts[1].References {
		if r.Kind == "publicacion" {
			ref = r
		}
	}
	if ref.DocID != 900000010 || ref.Pars != "5-5" || ref.Title != "Un artículo viejo" || ref.Extract != "2 Texto citado." || ref.Sync != "jwlib sync w --issue 19990101" {
		t.Errorf("reference %+v", ref)
	}
	if p := parts[2]; len(p.Questions) != 1 || p.Questions[0] != "Gé 1:26, 27. ¿Qué pregunta de prueba?" {
		t.Errorf("part 2 questions %q", p.Questions)
	}
	if p := parts[4]; p.Minutes != 15 || len(p.Videos) != 1 || p.Videos[0].Key != "pub-jwb-999_1_VIDEO" || p.Videos[0].Title != "" || len(p.Images) != 1 {
		t.Errorf("part 4 %+v", p)
	}
	sc := parts[5].Study
	if sc == nil || sc.DocID != 1102025901 || sc.Title != "Construyó el arca" || sc.Label != "1 NOÉ" ||
		len(sc.Accounts) != 1 || sc.Accounts[0] != "Génesis 6:9-22" || len(sc.Groups) != 1 || sc.Groups[0].Questions[0] != "¿Qué valor mostró Noé?" ||
		len(sc.Videos) != 1 || sc.Videos[0].Key != "pub-jwbai_201507_1_VIDEO" {
		t.Errorf("study chapter %+v", sc)
	}
	if len(w.Videos) != 2 {
		t.Errorf("videos %+v", w.Videos)
	}
	w.SetVideoTitle("pub-jwb-999_1_VIDEO", "Título del mediator", "3:21")
	if parts := w.Sections[len(w.Sections)-1].Parts; parts[0].Videos[0].Title != "Título del mediator" {
		t.Errorf("SetVideoTitle did not reach the part: %+v", parts[0].Videos)
	}

	docid, err := b.FindWatchtower(monday)
	if err != nil || docid != 2025801 {
		t.Fatalf("FindWatchtower = %d %v", docid, err)
	}
	if err := b.BuildWatchtower(w, docid); err != nil {
		t.Fatal(err)
	}
	wt := w.Watchtower
	if wt.Title != "Un artículo de estudio" || wt.Date != "5-11 DE ENERO DE 2026" || wt.ThemeRef != "Jn 17:3" || wt.Summary != "De qué trata." {
		t.Errorf("watchtower %+v", wt)
	}
	if len(wt.Songs) != 2 || wt.Songs[0].Number != 7 || wt.Songs[1].Number != 8 || wt.Songs[1].When != "final" {
		t.Errorf("wt songs %+v", wt.Songs)
	}
	if len(wt.Questions) != 2 || wt.Questions[1].Paragraphs != "2, 3" || wt.Questions[1].Subheading != "SUBTÍTULO" {
		t.Errorf("wt questions %+v", wt.Questions)
	}
	if len(wt.Review) != 1 || wt.Review[0] != "¿Repaso?" {
		t.Errorf("wt review %+v", wt.Review)
	}
}

func TestIssues(t *testing.T) {
	monday := time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)
	if got := WorkbookIssue(monday); got != "202609" {
		t.Errorf("WorkbookIssue = %s", got)
	}
	if got := WorkbookIssue(time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)); got != "202609" {
		t.Errorf("WorkbookIssue(oct) = %s", got)
	}
	if got := strings.Join(WatchtowerIssues(monday), ","); got != "202607,202608" {
		t.Errorf("WatchtowerIssues = %s", got)
	}
	if got := Monday(time.Date(2026, 10, 4, 12, 0, 0, 0, time.Local)).Format("2006-01-02"); got != "2026-09-28" {
		t.Errorf("Monday(sunday) = %s", got)
	}
}

// TestRealWeek2026_09_28 checks the acceptance facts against the real library
// when it has the September 2026 workbook (it is skipped elsewhere). The
// facts come from the 26-sep preparation run, made by hand on wol.
func TestRealWeek2026_09_28(t *testing.T) {
	dir := os.Getenv("JWLIB_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share", "jwlib")
	}
	if _, err := os.Stat(filepath.Join(dir, "pubs", "mwb_S_202609.jwpub")); err != nil {
		t.Skip("la biblioteca no tiene mwb_S_202609")
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b := &Builder{Store: st}
	monday := time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)
	dd, err := b.FindWorkbook(monday)
	if err != nil || dd == nil {
		t.Fatalf("FindWorkbook: %+v %v", dd, err)
	}
	w := &Week{}
	if err := b.BuildWorkbook(w, dd.DocID, *dd); err != nil {
		t.Fatal(err)
	}
	if w.Workbook.DocID != 202026255 {
		t.Errorf("Guía %d, want 202026255", w.Workbook.DocID)
	}
	if r := w.WeeklyReading; r.Book != "Jeremías" || len(r.Chapters) != 2 || r.Chapters[0] != 38 || r.Chapters[1] != 39 {
		t.Errorf("lectura semanal %+v", r)
	}
	var songs []int
	for _, s := range w.Songs {
		songs = append(songs, s.Number)
	}
	if len(songs) != 3 || songs[0] != 102 || songs[1] != 90 || songs[2] != 56 {
		t.Errorf("canciones %v, want 102/90/56", songs)
	}
	if sr := w.StudentReading; sr.Ref != "Jer 38:1-13" || sr.Lesson == nil || sr.Lesson.DocID != 1102018452 || !strings.Contains(sr.Lesson.Text, "th") || !strings.Contains(sr.Lesson.Text, "12") {
		t.Errorf("lectura del estudiante %+v %+v", sr, sr.Lesson)
	}
	if w.CongregationStudy == nil || w.CongregationStudy.DocID != 1102025910 || !strings.HasPrefix(w.CongregationStudy.Label, "10") {
		t.Errorf("wcg %+v", w.CongregationStudy)
	}
	found := false
	for _, v := range w.Videos {
		found = found || v.Key == "pub-jwb-125_4_VIDEO"
	}
	if !found {
		t.Errorf("falta el video de «¿Quién me tocó?»: %+v", w.Videos)
	}
	wanted := map[int]bool{2013043: false, 2019640: false, 2020562: false, 1102010147: false, 1102018452: false, 1102023309: false, 1102023301: false, 1102025910: false}
	for _, sec := range w.Sections {
		for _, p := range sec.Parts {
			for _, r := range p.References {
				if _, ok := wanted[r.DocID]; ok {
					wanted[r.DocID] = true
				}
			}
		}
	}
	for id, ok := range wanted {
		if !ok {
			t.Errorf("referencia %d no aparece", id)
		}
	}
}

func TestNoBreakSpaceHeadings(t *testing.T) {
	if m := songRe.FindStringSubmatch("Canción 128"); m == nil || m[1] != "128" {
		t.Errorf("songRe with NBSP: %v", m)
	}
	if m := minutesRe.FindStringSubmatch("(4 mins.)"); m == nil || m[1] != "4" {
		t.Errorf("minutesRe with NBSP: %v", m)
	}
	if got := partTitle("Canción 128"); got != "" {
		t.Errorf("a song-only heading is not a part: %q", got)
	}
}
