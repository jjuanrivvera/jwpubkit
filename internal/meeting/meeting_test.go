package meeting

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/testutil"
)

// weekHTML mirrors the markup of a Meeting Workbook week with invented text.
const weekHTML = `<header>
<h1 id="p1" data-pid="1">5-11 DE ENERO</h1>
<h2 id="p2" data-pid="2"><a href="jwpub://b/NWTR/1:1:1-1:1:31" class="b"><strong>GÉNESIS 1,</strong></a><a href="jwpub://b/NWTR/1:2:1-1:2:25" class="b"> <strong>2</strong></a></h2>
</header>
<div class="bodyTxt">
<h3 id="p3" data-pid="3"><a class="xt" href="jwpub://p/S:1102016801/"><strong>Canción 1</strong></a> <strong>y oración | Parte inicial de prueba</strong> <span>(1 min.)</span></h3>
<div class="du-fontSize--basePlus2 dc-icon--gem dc-icon-layout--top"><h2 id="p4" data-pid="4"><strong>PRIMERA SECCIÓN DE PRUEBA</strong></h2></div>
<div id="f1"><figure><img src="jwpub-media://900000001_univ_cnt_1.jpg" alt="Un paisaje." width="1200" height="675"/><figcaption><p id="p40" data-pid="40">Pie de prueba.</p></figcaption></figure></div>
<h3 id="p5" data-pid="5"><strong>1. Un título de prueba</strong></h3>
<div><p id="p6" data-pid="6">(10 mins.)</p></div>
<p id="p7" data-pid="7">Una idea (<a href="jwpub://b/NWTR/1:1:1-1:1:1" class="b">Gé 1:1</a>; <a class="xt" href="jwpub://p/S:900000010/5-5"><em>w99</em> 1/1 3 párr. 2</a>).</p>
<h3 id="p8" data-pid="8"><strong>2. Segunda parte de prueba</strong></h3>
<div><p id="p9" data-pid="9">(10 mins.)</p></div>
<ul><li><p id="p10" data-pid="10"><a href="jwpub://b/NWTR/1:1:26-1:1:27" class="b">Gé 1:26, 27</a>. ¿Qué pregunta de prueba?</p><div class="gen-field" id="p11" data-pid="11"><label>Respuesta</label><textarea></textarea></div></li></ul>
<h3 id="p12" data-pid="12"><strong>3. Lectura de la Biblia</strong></h3>
<div><p id="p13" data-pid="13">(4 mins.) <a href="jwpub://b/NWTR/1:2:1-1:2:9" class="b">Gé 2:1-9</a> (<a class="xt" href="jwpub://p/S:1102018445/"><em>th</em> lección 5</a>).</p></div>
<div class="dc-icon--sheep dc-icon-layout--top dc-icon-bgColor--red-600"><h2 id="p14" data-pid="14"><strong>SEGUNDA SECCIÓN DE PRUEBA</strong></h2></div>
<h3 id="p15" data-pid="15"><a class="xt" href="jwpub://p/S:1102016802/"><strong>Canción 2</strong></a></h3>
<h3 id="p16" data-pid="16"><strong>4. <em>“Un video”</em></strong></h3>
<div><p id="p17" data-pid="17">(15 mins.) Análisis con el auditorio.</p></div>
<div id="f2"><figure><img src="jwpub-media://900000001_univ_cnt_2.jpg" alt="Otra escena." width="1200" height="675"/></figure></div>
<p id="p18" data-pid="18"><a href="https://www.jw.org/finder?lank=pub-jwb-999_1_VIDEO&amp;wtlocale=S" data-video="webpubvid://?pub=jwb-999&amp;track=1&amp;langwritten=S"><strong>Ponga el VIDEO</strong></a>. Luego pregunte:</p>
<ul><li><p id="p19" data-pid="19">¿Qué aprendemos?</p><div class="gen-field" id="p20" data-pid="20"></div></li></ul>
<h3 id="p21" data-pid="21"><strong>5. Estudio bíblico de la congregación</strong></h3>
<div><p id="p22" data-pid="22">(30 mins.) <a class="xt" href="jwpub://p/S:1102025901/"><em>wcg</em> cap. 1</a>.</p></div>
<h3 id="p23" data-pid="23"><strong>Parte final de prueba</strong> <span>(3 mins.)</span> <strong>|</strong> <span><a class="xt" href="jwpub://p/S:1102016803/"><strong>Canción 3</strong></a></span> <strong>y oración</strong></h3>
</div>`

const chapterHTML = `<header><p class="contextTtl" id="p1" data-pid="1"><strong>1</strong> PERSONAJE DE PRUEBA</p>
<h1 id="p2" data-pid="2"><strong>Título de prueba del capítulo</strong></h1></header>
<div class="bodyTxt"><p id="p3" data-pid="3">Relato de prueba.</p>
<h3 id="p4" data-pid="4"><strong>Lea el relato bíblico</strong></h3>
<ul><li><p id="p5" data-pid="5"><a href="jwpub://b/NWTR/1:6:9-1:6:22" class="b"><strong>Génesis 6:9-22</strong></a></p></li></ul>
<h3 id="p6" data-pid="6"><strong>¿Pregunta de prueba?</strong></h3>
<p id="p7" data-pid="7"><strong>¿Segunda pregunta de prueba?</strong></p><div class="gen-field" id="p8" data-pid="8"></div>
<h2 id="p9" data-pid="9"><strong>Sección final de prueba</strong></h2>
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
		{DocID: 1, ExtractID: 1, Link: "p/S:1102016801/", Caption: caption("sjj canción 1", "Primera canción"), HTML: "<p>letra</p>", RefDocID: 1102016801, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 3, Sort: 1},
		{DocID: 1, ExtractID: 2, Link: "p/S:900000010/5-5", Caption: caption("w99 1/1 pág. 3", "Un artículo viejo"), HTML: `<p id="p5" data-pid="5"><span class="parNum" data-pnum="2"></span>Texto citado.</p>`, RefDocID: 900000010, RefSymbol: "w99", RefUndated: "w", RefIssue: 19990101, BeginPID: 7, Sort: 2},
		{DocID: 1, ExtractID: 3, Link: "p/S:1102018445/", Caption: caption("th pág. 8", "Leer con exactitud"), HTML: "<p>lección</p>", RefDocID: 1102018445, RefClass: 13, RefSymbol: "th", RefUndated: "th", BeginPID: 13, Sort: 3},
		{DocID: 1, ExtractID: 4, Link: "p/S:1102016802/", Caption: caption("sjj canción 2", "Segunda canción"), HTML: "<p>letra</p>", RefDocID: 1102016802, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 15, Sort: 4},
		{DocID: 1, ExtractID: 5, Link: "p/S:1102025901/", Caption: caption("wcg págs. 4-9", "Título de prueba del capítulo"), HTML: chapterHTML, RefDocID: 1102025901, RefClass: 13, RefSymbol: "wcg", RefUndated: "wcg", BeginPID: 22, Sort: 5},
		{DocID: 1, ExtractID: 6, Link: "p/S:1102016803/", Caption: caption("sjj canción 3", "Tercera canción"), HTML: "<p>letra</p>", RefDocID: 1102016803, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 23, Sort: 6},
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
	// The fixture is a Spanish workbook, so the references are read and printed
	// the way a Spanish library prints them.
	bible.UseLanguage("S")
	t.Cleanup(func() { bible.UseLanguage(bible.DefaultLang) })

	st, err := store.Open(t.TempDir(), "S")
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
	if strings.Join(songs, ",") != "start:Primera canción,middle:Segunda canción,end:Tercera canción" {
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
	if strings.Join(titles, "|") != "Parte inicial de prueba|Un título de prueba|Segunda parte de prueba|Lectura de la Biblia|“Un video”|Estudio bíblico de la congregación|Parte final de prueba" {
		t.Errorf("titles %q", titles)
	}
	if p := parts[1]; p.Minutes != 10 || len(p.Images) != 1 || p.Images[0].Caption != "Pie de prueba." {
		t.Errorf("part 1 %+v", p)
	}
	var ref Reference
	for _, r := range parts[1].References {
		if r.Kind == "publication" {
			ref = r
		}
	}
	if ref.DocID != 900000010 || ref.Pars != "5-5" || ref.Title != "Un artículo viejo" || ref.Extract != "2 Texto citado." || ref.Sync != "pubkit sync w --issue 19990101" {
		t.Errorf("reference %+v", ref)
	}
	if p := parts[2]; len(p.Questions) != 1 || p.Questions[0] != "Gé 1:26, 27. ¿Qué pregunta de prueba?" {
		t.Errorf("part 2 questions %q", p.Questions)
	}
	if p := parts[4]; p.Minutes != 15 || len(p.Videos) != 1 || p.Videos[0].Key != "pub-jwb-999_1_VIDEO" || p.Videos[0].Title != "" || len(p.Images) != 1 {
		t.Errorf("part 4 %+v", p)
	}
	sc := parts[5].Study
	if sc == nil || sc.DocID != 1102025901 || sc.Title != "Título de prueba del capítulo" || sc.Label != "1 PERSONAJE DE PRUEBA" ||
		len(sc.Accounts) != 1 || sc.Accounts[0] != "Génesis 6:9-22" || len(sc.Groups) != 1 || sc.Groups[0].Questions[0] != "¿Segunda pregunta de prueba?" ||
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
	if len(wt.Songs) != 2 || wt.Songs[0].Number != 7 || wt.Songs[1].Number != 8 || wt.Songs[1].When != "end" {
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

// Publications separate a number from its unit with a no-break space, and write
// the number in their own script. Both have to survive the readers.
func TestNoBreakSpaceHeadings(t *testing.T) {
	if n, ok := firstNumber("Canci\u00f3n\u00a0128"); !ok || n != 128 {
		t.Errorf("a no-break space before the number: %d %v", n, ok)
	}
	if n, ok := minutesIn("(4\u00a0mins.)"); !ok || n != 4 {
		t.Errorf("minutes with a no-break space: %d %v", n, ok)
	}
	if n, ok := minutesIn("\uff08\uff14\u00a0\u5206\uff09"); !ok || n != 4 {
		t.Errorf("minutes in fullwidth digits: %d %v", n, ok)
	}
}

// A heading that only announces a song is not a part of the meeting, and that
// has to hold whatever the word for "song" is: the song is recognised by its own
// link text, not by the sentence around it.
func TestSongOnlyHeadingIsNotAPart(t *testing.T) {
	for _, c := range []struct{ name, heading, link string }{
		{"spanish", "Canci\u00f3n 128 y oraci\u00f3n", "Canci\u00f3n 128"},
		{"english", "Song 128 and prayer", "Song 128"},
		{"japanese", "\u6b4c 128\u3001\u7948\u308a", "\u6b4c 128"},
		{"arabic", "\u0627\u0644\u062a\u0631\u0646\u064a\u0645\u0629 \u0661\u0662\u0668 \u0648\u0627\u0644\u0635\u0644\u0627\u0629", "\u0627\u0644\u062a\u0631\u0646\u064a\u0645\u0629 \u0661\u0662\u0668"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := partTitle(c.heading, []string{c.link}); got != "" {
				t.Errorf("partTitle(%q) = %q, want it dropped", c.heading, got)
			}
		})
	}
	got := partTitle("Canci\u00f3n 128 y oraci\u00f3n | T\u00edtulo de la parte (1 min.)", []string{"Canci\u00f3n 128"})
	if got != "T\u00edtulo de la parte" {
		t.Errorf("partTitle kept %q", got)
	}
}

// The claim this parser makes is that it reads a week by its structure, not by
// its words. The way to test that claim is to render the same structure with
// text and digits from scripts that share nothing, and require the same answer.
// If any of these ever diverge, some language-specific assumption crept back in.
func TestSameStructureAcrossScripts(t *testing.T) {
	type lang struct {
		name                          string
		section1, section2            string
		song1, song2, song3           string
		part1, reading, study, video  string
		mins10, mins4, mins15, mins30 string
	}
	langs := []lang{{
		name: "latin", section1: "FIRST SECTION", section2: "SECOND SECTION",
		song1: "Song 1", song2: "Song 2", song3: "Song 3",
		part1: "1. A part", reading: "3. Bible reading", study: "5. Congregation study",
		video: "4. “A video”", mins10: "(10 mins.)", mins4: "(4 mins.)",
		mins15: "(15 mins.)", mins30: "(30 mins.)",
	}, {
		name: "arabic", section1: "القسم الأول", section2: "القسم الثاني",
		song1: "الترنيمة ١", song2: "الترنيمة ٢", song3: "الترنيمة ٣",
		part1: "١- جزء", reading: "٣- قراءة", study: "٥- درس", video: "٤- «فيديو»",
		mins10: "(١٠ دقائق)", mins4: "(٤ دقائق)", mins15: "(١٥ دقيقة)", mins30: "(٣٠ دقيقة)",
	}, {
		name: "cjk", section1: "第一部分", section2: "第二部分",
		song1: "歌 1", song2: "歌 2", song3: "歌 3",
		part1: "1．一个部分", reading: "3．圣经朗读", study: "5．会众研究", video: "4．「视频」",
		mins10: "（10分）", mins4: "（4分）", mins15: "（15分）", mins30: "（30分）",
	}}

	var want *Week
	for _, l := range langs {
		t.Run(l.name, func(t *testing.T) {
			st, err := store.Open(t.TempDir(), "X")
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			pub := scriptWorkbook(l.section1, l.section2, l.song1, l.song2, l.song3,
				l.part1, l.reading, l.study, l.video, l.mins10, l.mins4, l.mins15, l.mins30)
			if _, err := st.IndexLocal(testutil.Build(t, t.TempDir(), pub), "mwb", "202601", "X"); err != nil {
				t.Fatal(err)
			}
			b := &Builder{Store: st}
			dd, err := b.FindWorkbook(time.Date(2026, 1, 5, 0, 0, 0, 0, time.Local))
			if err != nil || dd == nil {
				t.Fatalf("FindWorkbook: %+v %v", dd, err)
			}
			w := &Week{Monday: "2026-01-05"}
			if err := b.BuildWorkbook(w, dd.DocID, *dd); err != nil {
				t.Fatal(err)
			}

			// The structural answers, which must not depend on the script.
			if len(w.Songs) != 3 {
				t.Fatalf("songs %+v", w.Songs)
			}
			for i, n := range []int{1, 2, 3} {
				if w.Songs[i].Number != n {
					t.Errorf("song %d has number %d", i, w.Songs[i].Number)
				}
			}
			if w.StudentReading == nil || w.StudentReading.Range == nil {
				t.Errorf("student reading not identified: %+v", w.StudentReading)
			}
			if w.CongregationStudy == nil {
				t.Error("congregation study not identified")
			}
			var mins []int
			var nums []int
			for _, sec := range w.Sections {
				for _, p := range sec.Parts {
					mins = append(mins, p.Minutes)
					nums = append(nums, p.Number)
				}
			}
			got := &Week{Songs: w.Songs, Sections: w.Sections}
			if want == nil {
				want = got
				if fmt.Sprint(mins) != "[10 4 15 30]" {
					t.Errorf("minutes %v, want [10 4 15 30]", mins)
				}
				if fmt.Sprint(nums) != "[1 3 4 5]" {
					t.Errorf("part numbers %v, want [1 3 4 5]", nums)
				}
				return
			}
			if fmt.Sprint(mins) != "[10 4 15 30]" {
				t.Errorf("minutes %v differ from the latin rendering", mins)
			}
			if fmt.Sprint(nums) != "[1 3 4 5]" {
				t.Errorf("part numbers %v differ from the latin rendering", nums)
			}
			if len(w.Sections) != len(want.Sections) {
				t.Errorf("section count %d, want %d", len(w.Sections), len(want.Sections))
			}
		})
	}
}

// scriptWorkbook renders the fixture's structure with whatever words it is
// given, so the same skeleton can be tried in several scripts.
func scriptWorkbook(sec1, sec2, song1, song2, song3, part1, reading, study, video,
	m10, m4, m15, m30 string) testutil.Pub {
	html := `<header><h1 id="p1" data-pid="1">WEEK</h1>
<h2 id="p2" data-pid="2"><a href="jwpub://b/NWTR/1:1:1-1:1:31" class="b">GENESIS 1</a></h2></header>
<div class="bodyTxt">
<h3 id="p3" data-pid="3"><a class="xt" href="jwpub://p/S:1102016801/"><strong>` + song1 + `</strong></a></h3>
<div class="dc-icon--gem dc-icon-layout--top"><h2 id="p4" data-pid="4"><strong>` + sec1 + `</strong></h2></div>
<h3 id="p5" data-pid="5"><strong>` + part1 + `</strong></h3>
<div><p id="p6" data-pid="6">` + m10 + `</p></div>
<h3 id="p12" data-pid="12"><strong>` + reading + `</strong></h3>
<div><p id="p13" data-pid="13">` + m4 + ` <a href="jwpub://b/NWTR/1:2:1-1:2:9" class="b">Ge 2:1-9</a></p></div>
<div class="dc-icon--sheep dc-icon-layout--top"><h2 id="p14" data-pid="14"><strong>` + sec2 + `</strong></h2></div>
<h3 id="p15" data-pid="15"><a class="xt" href="jwpub://p/S:1102016802/"><strong>` + song2 + `</strong></a></h3>
<h3 id="p16" data-pid="16"><strong>` + video + `</strong></h3>
<div><p id="p17" data-pid="17">` + m15 + `</p></div>
<h3 id="p21" data-pid="21"><strong>` + study + `</strong></h3>
<div><p id="p22" data-pid="22">` + m30 + ` <a class="xt" href="jwpub://p/S:1102025901/">chapter</a></p></div>
<h3 id="p23" data-pid="23"><a class="xt" href="jwpub://p/S:1102016803/"><strong>` + song3 + `</strong></a></h3>
</div>`
	chapter := `<header><p class="contextTtl" id="p1" data-pid="1"><strong>1</strong> NAME</p>
<h1 id="p2" data-pid="2"><strong>Chapter title</strong></h1></header>
<div class="bodyTxt"><p id="p3" data-pid="3">A paragraph.</p>
<h3 id="p4" data-pid="4"><strong>Accounts</strong></h3>
<ul><li><p id="p5" data-pid="5"><a href="jwpub://b/NWTR/1:6:9-1:6:22" class="b"><strong>Ge 6:9-22</strong></a></p></li></ul>
<h3 id="p6" data-pid="6"><strong>Questions</strong></h3>
<p id="p7" data-pid="7"><strong>A question?</strong></p><div class="gen-field" id="p8" data-pid="8"></div>
</div>`
	return testutil.Pub{
		Symbol: "mwb26", Undated: "mwb", Year: 2026, IssueTag: 20260100, Title: "Workbook",
		Docs:  []testutil.Doc{{ID: 1, MepsID: 202026001, Class: 106, Title: "week", HTML: html}},
		Dated: [][4]any{{1, 20260105, 20260111, "p/S:202026001/1-23"}},
		Extracts: []testutil.Extract{
			{DocID: 1, ExtractID: 1, Link: "p/S:1102016801/", Caption: caption("s 1", "One"), HTML: "<p>x</p>", RefDocID: 1102016801, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 3, Sort: 1},
			{DocID: 1, ExtractID: 4, Link: "p/S:1102016802/", Caption: caption("s 2", "Two"), HTML: "<p>x</p>", RefDocID: 1102016802, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 15, Sort: 2},
			{DocID: 1, ExtractID: 5, Link: "p/S:1102025901/", Caption: caption("b 4-9", "Chapter title"), HTML: chapter, RefDocID: 1102025901, RefClass: 13, RefSymbol: "wcg", RefUndated: "wcg", BeginPID: 22, Sort: 3},
			{DocID: 1, ExtractID: 6, Link: "p/S:1102016803/", Caption: caption("s 3", "Three"), HTML: "<p>x</p>", RefDocID: 1102016803, RefClass: 31, RefSymbol: "sjj", RefUndated: "sjj", BeginPID: 23, Sort: 4},
		},
	}
}
