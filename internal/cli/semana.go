package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/meeting"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

func (a *app) semanaCmd() *cobra.Command {
	var noWT, extracts bool
	cmd := &cobra.Command{
		Use:     "semana [AAAA-MM-DD]",
		Aliases: []string{"week"},
		Short:   "La reunión de entre semana y La Atalaya de estudio de esa semana",
		Long: `Toma el lunes de la semana de la fecha (hoy si no se indica) y arma, desde la Guía de
actividades: docid, lectura bíblica semanal, lectura del estudiante y su lección de
"Seamos mejores maestros", canciones, cada parte con su título, tiempo, preguntas,
referencias (citas y docids, con el texto que la Guía trae de cada referencia), videos
(con su clave para "jwlib subtitulos") e imágenes. El estudio bíblico de la congregación
incluye el capítulo completo que trae la Guía: título, relatos, preguntas y videos.

Si existe, agrega La Atalaya de estudio de esa semana: docid, título, texto temático,
canciones y preguntas por párrafo. Si falta la Guía o La Atalaya en la biblioteca, las
sincroniza (salvo con --sin-red).`,
		Example: `  jwlib semana 2026-09-28
  jwlib semana 2026-09-30 --extractos
  jwlib semana --json 2026-09-28 | jq '.secciones[].partes[] | {numero, titulo, minutos}'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			day := time.Now()
			if len(args) == 1 {
				d, err := time.ParseInLocation("2006-01-02", args[0], time.Local)
				if err != nil {
					return fmt.Errorf("fecha inválida %q (usa AAAA-MM-DD)", args[0])
				}
				day = d
			}
			w, err := a.buildWeek(day, !noWT)
			if err != nil {
				return err
			}
			if a.jsonOut {
				w.Normalize()
				return a.printJSON(w) // always with extracts: they save the trips to wol
			}
			a.printWeek(w, extracts)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noWT, "sin-atalaya", false, "no incluir La Atalaya de estudio")
	cmd.Flags().BoolVar(&extracts, "extractos", false, "mostrar el texto de cada referencia que trae la Guía")
	return cmd
}

func (a *app) buildWeek(day time.Time, withWT bool) (*meeting.Week, error) {
	st, err := a.store()
	if err != nil {
		return nil, err
	}
	monday := meeting.Monday(day)
	w := &meeting.Week{Monday: monday.Format("2006-01-02")}
	b := &meeting.Builder{Store: st}

	dd, err := b.FindWorkbook(monday)
	if err != nil {
		return nil, err
	}
	if dd == nil {
		issue := meeting.WorkbookIssue(monday)
		if a.offline {
			return nil, fmt.Errorf("la Guía de la semana del %s no está en la biblioteca (jwlib sync mwb --issue %s)", w.Monday, issue)
		}
		a.logf("la Guía mwb %s no está en la biblioteca; sincronizando", issue)
		if _, err := a.syncOne("mwb", issue, false); err != nil {
			return nil, fmt.Errorf("sincronizando la Guía mwb %s: %w", issue, err)
		}
		if dd, err = b.FindWorkbook(monday); err != nil {
			return nil, err
		}
		if dd == nil {
			return nil, fmt.Errorf("la Guía mwb %s no trae la semana del %s (¿asamblea, Conmemoración o visita?)", issue, w.Monday)
		}
	}
	if err := b.BuildWorkbook(w, dd.DocID, *dd); err != nil {
		return nil, err
	}

	if withWT {
		docid, err := b.FindWatchtower(monday)
		if err != nil {
			return nil, err
		}
		if docid == 0 && !a.offline {
			for _, issue := range meeting.WatchtowerIssues(monday) {
				if p, _ := st.PubByKey(store.PubKey("w", a.lang, issue)); p != nil {
					continue
				}
				a.logf("buscando La Atalaya de estudio de la semana en w %s", issue)
				if _, err := a.syncOne("w", issue, false); err != nil {
					a.logf("w %s: %v", issue, err)
					continue
				}
				if docid, err = b.FindWatchtower(monday); err != nil {
					return nil, err
				}
				if docid != 0 {
					break
				}
			}
		}
		if docid != 0 {
			if err := b.BuildWatchtower(w, docid); err != nil {
				return nil, err
			}
		} else {
			w.Notes = append(w.Notes, "No se encontró La Atalaya de estudio para esta semana (probadas: w "+strings.Join(meeting.WatchtowerIssues(monday), ", ")+").")
		}
	}
	a.resolveVideos(w)
	return w, nil
}

// resolveVideos fills titles and durations from the mediator API, cached in
// the library so the next run needs no network.
func (a *app) resolveVideos(w *meeting.Week) {
	st, _ := a.store()
	for _, key := range w.SortedVideoKeys() {
		title, dur, err := a.videoInfo(st, key)
		if err != nil {
			if !errors.Is(err, errOffline) {
				w.Notes = append(w.Notes, fmt.Sprintf("Video %s: %v", key, err))
			}
			continue
		}
		w.SetVideoTitle(key, title, dur)
	}
}

func (a *app) videoInfo(st *store.Store, key string) (title, dur string, err error) {
	if v, _ := st.Video(key, a.lang); v != nil {
		return v.Title, subs.FormatTS(time.Duration(v.Duration * float64(time.Second))), nil
	}
	if a.offline {
		return "", "", errOffline
	}
	item, err := a.client().MediaItem(a.ctx, key)
	if err != nil {
		return "", "", err
	}
	_ = st.PutVideo(key, a.lang, item.Title, item.Duration, item.SubtitlesURL(), item)
	return item.Title, subs.FormatTS(time.Duration(item.Duration * float64(time.Second))), nil
}

func (a *app) printWeek(w *meeting.Week, extracts bool) {
	p := a.printf
	p("Semana del %s (lunes %s)\n", w.Range, w.Monday)
	if g := w.Workbook; g != nil {
		p("Guía de actividades: docid %d · %s · %s\n", g.DocID, g.Location, g.URL)
	}
	if r := w.WeeklyReading; r != nil {
		p("Lectura bíblica semanal: %s (%s)\n", r.Text, r.Ref)
	}
	if sr := w.StudentReading; sr != nil {
		line := "Lectura del estudiante: " + sr.Ref
		if l := sr.Lesson; l != nil {
			line += fmt.Sprintf(" · %s «%s» (docid %d)", l.Text, l.Title, l.DocID)
		}
		p("%s\n", line)
	}
	if len(w.Songs) > 0 {
		var ss []string
		for _, s := range w.Songs {
			ss = append(ss, fmt.Sprintf("%d «%s» (%s)", s.Number, s.Title, s.When))
		}
		p("Canciones: %s\n", strings.Join(ss, " · "))
	}
	for _, sec := range w.Sections {
		p("\n")
		if sec.Title != "" {
			p("%s\n", sec.Title)
		}
		for _, part := range sec.Parts {
			a.printPart(part, extracts)
		}
	}
	if len(w.Videos) > 0 {
		p("\nVideos de la reunión (transcripción: jwlib subtitulos <clave>)\n")
		seen := map[string]bool{}
		for _, v := range w.Videos {
			if seen[v.Key] {
				continue
			}
			seen[v.Key] = true
			p("  %-28s %s", v.Key, v.Title)
			if v.Duration != "" {
				p(" (%s)", v.Duration)
			}
			if v.Part != "" {
				p(" · parte %s", v.Part)
			}
			p("\n")
		}
	}
	if wt := w.Watchtower; wt != nil {
		p("\nLA ATALAYA DE ESTUDIO · %s\n", wt.Date)
		p("«%s» · docid %d · %s · %s\n", wt.Title, wt.DocID, wt.Location, wt.URL)
		if wt.Theme != "" {
			p("Texto temático: %s\n", wt.Theme)
		}
		if wt.Summary != "" {
			p("Tema: %s\n", wt.Summary)
		}
		for _, s := range wt.Songs {
			p("Canción %d «%s» (%s)\n", s.Number, s.Title, s.When)
		}
		p("Preguntas:\n")
		sub := ""
		for _, q := range wt.Questions {
			if q.Subheading != sub && q.Subheading != "" {
				p("  %s\n", q.Subheading)
				sub = q.Subheading
			}
			p("    %s. %s\n", q.Paragraphs, q.Text)
		}
		for _, box := range wt.Boxes {
			p("  Recuadro «%s»:\n", box.Title)
			for _, q := range box.Questions {
				p("    - %s\n", q)
			}
		}
		if len(wt.Images) > 0 {
			p("  Imágenes (jwlib imagen %d):\n", wt.DocID)
			for _, im := range wt.Images {
				p("    %s · %s\n", im.File, firstNonEmpty(im.Caption, im.Alt))
			}
		}
	}
	for _, n := range w.Notes {
		p("\nAviso: %s\n", n)
	}
}

func (a *app) printPart(part meeting.Part, extracts bool) {
	p := a.printf
	head := part.Title
	if part.Number > 0 {
		head = fmt.Sprintf("%d. %s", part.Number, part.Title)
	}
	if part.Minutes > 0 {
		head += fmt.Sprintf(" (%d min.)", part.Minutes)
	}
	p("  %s\n", head)
	for _, t := range part.Text {
		p("     %s\n", t)
	}
	for _, q := range part.Questions {
		p("     ? %s\n", q)
	}
	for _, r := range part.References {
		if r.Kind == "biblia" {
			continue
		}
		loc := firstNonEmpty(r.Location, r.Text)
		line := fmt.Sprintf("     → %s", loc)
		if r.Title != "" {
			line += " «" + r.Title + "»"
		}
		line += fmt.Sprintf(" · docid %d", r.DocID)
		if r.Pars != "" {
			line += " ¶" + r.Pars
		}
		if !r.InLibrary && r.Sync != "" {
			line += " · " + r.Sync
		}
		p("%s\n", line)
		if extracts && r.Extract != "" && part.Study == nil {
			for _, l := range strings.Split(r.Extract, "\n") {
				p("         %s\n", l)
			}
		}
	}
	var bibleRefs []bible.Range
	for _, r := range part.References {
		if r.Kind == "biblia" && r.Range != nil {
			bibleRefs = append(bibleRefs, *r.Range)
		}
	}
	if len(bibleRefs) > 0 {
		p("     Textos: %s\n", bible.FormatList(bibleRefs))
	}
	for _, v := range part.Videos {
		p("     Video: %s %s", v.Key, v.Title)
		if v.Duration != "" {
			p(" (%s)", v.Duration)
		}
		p("\n")
	}
	for _, im := range part.Images {
		p("     Imagen: %s · %s\n", im.File, firstNonEmpty(im.Caption, im.Alt))
	}
	if sc := part.Study; sc != nil {
		p("     Capítulo: %s · «%s» · %s · docid %d\n", sc.Label, sc.Title, sc.Location, sc.DocID)
		if len(sc.Accounts) > 0 {
			p("     Relato bíblico: %s\n", strings.Join(sc.Accounts, "; "))
		}
		for _, g := range sc.Groups {
			p("     %s\n", g.Title)
			for _, q := range g.Questions {
				p("       ? %s\n", q)
			}
		}
		for _, r := range sc.References {
			p("       → %s · docid %d ¶%s\n", r.Text, r.DocID, r.Pars)
		}
		for _, v := range sc.Videos {
			p("     Video: %s %s", v.Key, v.Title)
			if v.Duration != "" {
				p(" (%s)", v.Duration)
			}
			p("\n")
		}
		if len(sc.Images) > 0 {
			p("     Imágenes del capítulo: %d (jwlib imagen %d)\n", len(sc.Images), sc.DocID)
		}
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
