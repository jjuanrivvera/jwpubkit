package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

// targetChapterTokens is the token budget a chapter .md aims to stay under,
// so a single file comfortably fits in one LLM context window on its own.
// Tokens are estimated with the rough heuristic chars/4 (documented in
// tokenEstimate), not a real tokenizer: it is meant to catch chapters whose
// citation extracts have grown huge, not to be exact.
const targetChapterTokens = 6000

// extractCapSteps are the extract lengths (runes) tried, in order, when a
// chapter's estimate is over budget. 0 means "no cap": most chapters render
// their citation extracts at full length on this first pass and never need
// the smaller steps. Verses and citations are never dropped, only the
// quoted paragraph shrinks.
var extractCapSteps = []int{0, 600, 350, 200, 120}

// tokenEstimate is the chars/4 heuristic used to size dossiers: rough, but
// good enough to flag a chapter that needs trimming before an agent reads it.
func tokenEstimate(s string) int { return (len([]rune(s)) + 3) / 4 }

// citationOut is one library document/paragraph that cites a verse of the
// chapter, deduplicated across verses.
type citationOut struct {
	DocID     int    `json:"docid"`
	Pub       string `json:"publication"`
	Title     string `json:"title"`
	PID       int    `json:"pid"`
	URL       string `json:"url"`
	Year      int    `json:"year,omitempty"`
	Verses    []int  `json:"verses"`
	Extract   string `json:"extract"`
	Trimmed   bool   `json:"extract_trimmed,omitempty"`
	NoExtract bool   `json:"no_extract,omitempty"`
}

// imageOut is one image found on a document referenced by the chapter's
// citations, with the size recorded in the library (no network call: see
// the package doc comment on why CDN sizes are not compared here).
type imageOut struct {
	DocID   int    `json:"docid"`
	Num     int    `json:"number"`
	File    string `json:"file"`
	Caption string `json:"caption,omitempty"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// placeOut is a proper-noun candidate found in the chapter's study notes
// and footnotes, cross-referenced against Perspicacia (it) by exact title.
type placeOut struct {
	Name    string `json:"name"`
	ItDocID int    `json:"it_docid,omitempty"`
	InIt    bool   `json:"in_it"`
}

// chapterDossier is everything expediente gathers for one chapter.
type chapterDossier struct {
	Book        int           `json:"book"`
	Chapter     int           `json:"chapter"`
	Ref         string        `json:"reference"`
	Verses      []store.Verse `json:"verses"`
	Citations   []citationOut `json:"citations"`
	Images      []imageOut    `json:"images"`
	Places      []placeOut    `json:"places"`
	NotInLib    []string      `json:"not_in_library"`
	VerseCount  int           `json:"verse_count"`
	CiteCount   int           `json:"citation_count"`
	ImageCount  int           `json:"image_count"`
	PlaceCount  int           `json:"place_count"`
	TokenEst    int           `json:"estimated_tokens"`
	ExtractsCut bool          `json:"extracts_trimmed,omitempty"`
	File        string        `json:"file,omitempty"`
}

type dossierResult struct {
	Ref      string            `json:"reference"`
	OutDir   string            `json:"output_dir"`
	Chapters []*chapterDossier `json:"chapters"`
}

func (a *app) dossierCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:     `dossier "<chapter-reference>"`,
		Aliases: []string{"expediente"},
		Short:   "A per-chapter dossier (text, citations, images, places) meant to be read once",
		Long: `Builds, entirely from the local library and without calling any model, one dossier per
CHAPTER — meant for an agent preparing a talk or a video to read once instead of
walking a website across many calls. For every chapter in the range:

  1. The Bible text with its study notes, footnotes and marginal references, as
     faithfully as "pubkit verse" renders them.
  2. The library documents that cite each verse (the BibleCitation table), with the
     extract of the citing paragraph, deduplicated by document and paragraph and
     carrying the verses each one cites. They are ordered by how many distinct verses
     of the chapter they cite and, on a tie, by the most recent publication.
  3. The images of those citing documents, at the size the library holds (nothing is
     downloaded from the CDN here).
  4. Proper nouns from the chapter's notes and footnotes, cross-referenced by exact
     title against an encyclopedic publication (it) when it is synced.
  5. An index with the counts and an explicit list of what is NOT in the library, so
     the agent knows what to look for elsewhere instead of assuming it does not exist.

Writes one .md per chapter and one .json with all of them into --output (the current
directory by default). Each .md ends with a token estimate (the chars/4 heuristic);
when a chapter goes over budget the citation extracts are trimmed — verses and
citations are never dropped.`,
		Example: `  pubkit dossier "Gen 37-41" --output /tmp/dossier
  pubkit dossier "Jer 38-39" --json | jq '.chapters[].citation_count'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			if !st.HasBible() {
				if a.offline {
					return fmt.Errorf("no study Bible in the library: pubkit sync nwtsty")
				}
				a.logf("no study Bible in the library (nwtsty, ~127 MB); syncing it")
				if _, err := a.syncOne("nwtsty", "", false); err != nil {
					return err
				}
			}
			chapters, err := chapterRanges(args[0])
			if err != nil {
				return err
			}
			if outDir == "" {
				outDir = "."
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			hasIt := st.HasSymbol("it")
			if !hasIt {
				a.logf("the encyclopedic publication (it) is not in the library; the place cross-reference is skipped")
			}
			res := &dossierResult{Ref: bible.FormatList(chapters), OutDir: outDir}
			for _, r := range chapters {
				a.logf("building the dossier for %s", r.Long())
				d, err := a.buildChapterDossier(st, r, hasIt)
				if err != nil {
					return err
				}
				md := renderChapterDossier(d)
				name := fmt.Sprintf("dossier_%s_%d.md", chapterSlug(r.Book), r.StartChapter)
				path := filepath.Join(outDir, name)
				if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
					return err
				}
				d.File = path
				res.Chapters = append(res.Chapters, d)
				a.logf("%s · %d verses · %d citations · %d images · %d places · ~%d tokens → %s",
					r.Long(), d.VerseCount, d.CiteCount, d.ImageCount, d.PlaceCount, d.TokenEst, path)
			}
			jsonPath := filepath.Join(outDir, "dossier.json")
			jf, err := os.Create(jsonPath)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(jf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(res); err != nil {
				jf.Close()
				return err
			}
			if err := jf.Close(); err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(res)
			}
			a.printf("Dossier for %s · %d chapters · directory %s\n", res.Ref, len(res.Chapters), outDir)
			for _, d := range res.Chapters {
				a.printf("  %s · %d verses · %d citations · %d images · %d places · ~%d tokens · %s\n",
					d.Ref, d.VerseCount, d.CiteCount, d.ImageCount, d.PlaceCount, d.TokenEst, filepath.Base(d.File))
			}
			a.printf("  %s\n", jsonPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outDir, "output", "o", "", "directory for the .md files and the .json (default: the current one)")
	return cmd
}

// chapterRanges expands a chapter reference ("Gen 37-41", "Jer 38-39") into
// one whole-chapter bible.Range per chapter, in order, without repeats. It
// reuses bible.Parse per chapter (via "<book> <chapter>") instead of
// reimplementing chapter-boundary rules (Psalm superscriptions, etc.).
func chapterRanges(ref string) ([]bible.Range, error) {
	ranges, err := bible.Parse(ref)
	if err != nil {
		return nil, err
	}
	var out []bible.Range
	seen := map[[2]int]bool{}
	for _, r := range ranges {
		for c := r.StartChapter; c <= r.EndChapter; c++ {
			key := [2]int{r.Book, c}
			if seen[key] {
				continue
			}
			seen[key] = true
			bk, ok := bible.BookByNum(r.Book)
			if !ok {
				return nil, fmt.Errorf("unknown book %d", r.Book)
			}
			crs, err := bible.Parse(fmt.Sprintf("%s %d", bk.Short, c))
			if err != nil {
				return nil, err
			}
			out = append(out, crs[0])
		}
	}
	return out, nil
}

// chapterSlug is an ASCII, filesystem-friendly form of a book's short name.
func chapterSlug(book int) string {
	bk, _ := bible.BookByNum(book)
	return foldASCII(strings.ReplaceAll(bk.Short, " ", ""))
}

var asciiReplacer = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n",
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ñ", "N",
)

func foldASCII(s string) string { return asciiReplacer.Replace(s) }

// buildChapterDossier gathers everything expediente knows about one chapter
// from the local library: no network call is ever made here.
func (a *app) buildChapterDossier(st *store.Store, r bible.Range, hasIt bool) (*chapterDossier, error) {
	d := &chapterDossier{Book: r.Book, Chapter: r.StartChapter, Ref: r.Long()}
	verses, err := st.Verses(r.FirstID(), r.LastID())
	if err != nil {
		return nil, err
	}
	if len(verses) == 0 {
		return nil, fmt.Errorf("%s is not in the library's Bible", r.Long())
	}
	for i := range verses {
		for j := range verses[i].XRefs {
			x := &verses[i].XRefs[j]
			var rs []bible.Range
			for _, t := range x.Targets {
				if rg, ok := bible.FromIDs(t[0], t[1]); ok {
					rs = append(rs, rg)
				}
			}
			x.Refs = strings.Split(bible.FormatList(rs), "; ")
		}
	}
	d.Verses = verses
	d.VerseCount = len(verses)

	cites, missingCite, err := a.chapterCitations(st, verses)
	if err != nil {
		return nil, err
	}
	d.Citations = cites
	d.CiteCount = len(cites)
	d.NotInLib = append(d.NotInLib, missingCite...)

	images, missingImg := a.chapterImages(st, cites)
	d.Images = images
	d.ImageCount = len(images)
	d.NotInLib = append(d.NotInLib, missingImg...)

	if hasIt {
		d.Places = a.chapterPlaces(st, verses)
	} else {
		d.NotInLib = append(d.NotInLib, "the encyclopedic publication (it) is not synced: the places were not cross-referenced")
	}
	d.PlaceCount = len(d.Places)

	for _, v := range verses {
		for _, n := range v.Notes {
			if n.DocID == 0 {
				continue
			}
			if _, err := st.Doc(n.DocID); err != nil {
				d.NotInLib = append(d.NotInLib, fmt.Sprintf("a study note on %s points at docid %d, which is not synced", verseLabel(v), n.DocID))
			}
		}
	}
	d.NotInLib = dedupStrings(d.NotInLib)
	return d, nil
}

func verseLabel(v store.Verse) string {
	bk, _ := bible.BookByNum(v.Book)
	return fmt.Sprintf("%s %d:%d", bk.Short, v.Chapter, v.Verse)
}

// chapterCitations gathers, per (docid, pid), every distinct verse of the
// chapter it cites, then attaches the paragraph extract and sorts by
// relevance: (a) distinct verses cited, (b) newer publications first.
func (a *app) chapterCitations(st *store.Store, verses []store.Verse) ([]citationOut, []string, error) {
	type key struct{ docid, pid int }
	entries := map[key]*citationOut{}
	var order []key
	for _, v := range verses {
		if v.Verse == 0 {
			continue
		}
		cites, _, err := st.CitedBy(v.ID, v.ID, 0)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range cites {
			pids := c.PIDs
			if len(pids) == 0 {
				pids = []int{0}
			}
			for _, pid := range pids {
				k := key{c.DocID, pid}
				e, ok := entries[k]
				if !ok {
					e = &citationOut{DocID: c.DocID, Pub: c.Pub, Title: c.Title, PID: pid, URL: c.URL, Year: c.Year}
					entries[k] = e
					order = append(order, k)
				}
				if !containsInt(e.Verses, v.Verse) {
					e.Verses = append(e.Verses, v.Verse)
				}
			}
		}
	}
	var missing []string
	out := make([]citationOut, 0, len(order))
	for _, k := range order {
		e := entries[k]
		sort.Ints(e.Verses)
		if k.pid > 0 {
			text, err := st.ParagraphText(k.docid, k.pid)
			if err != nil {
				return nil, nil, err
			}
			if text == "" {
				e.NoExtract = true
				missing = append(missing, fmt.Sprintf("docid %d pid %d: the citation has no indexed extract", k.docid, k.pid))
			}
			e.Extract = text
		} else {
			e.NoExtract = true
		}
		out = append(out, *e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Verses) != len(out[j].Verses) {
			return len(out[i].Verses) > len(out[j].Verses)
		}
		if out[i].Year != out[j].Year {
			return out[i].Year > out[j].Year
		}
		return out[i].DocID < out[j].DocID
	})
	return out, missing, nil
}

// chapterImages lists the images of the documents cited in the chapter, with
// the width/height the library recorded when the JWPUB was indexed. No
// network call: comparing against the CDN (as "pubkit image" does) would
// need to download bytes, which expediente is built to avoid entirely.
func (a *app) chapterImages(st *store.Store, cites []citationOut) ([]imageOut, []string) {
	seenDocs := map[int]bool{}
	var out []imageOut
	var missing []string
	for _, c := range cites {
		if seenDocs[c.DocID] {
			continue
		}
		seenDocs[c.DocID] = true
		media, err := st.MediaOf(c.DocID)
		if err != nil {
			missing = append(missing, fmt.Sprintf("docid %d: its images could not be read (%v)", c.DocID, err))
			continue
		}
		n := 0
		for _, m := range media {
			if m.Width == 0 || m.Height == 0 || !strings.HasPrefix(m.Mime, "image") {
				continue
			}
			n++
			out = append(out, imageOut{
				DocID: c.DocID, Num: n, File: m.File,
				Caption: firstNonEmpty(m.Caption, m.Label),
				Width:   m.Width, Height: m.Height,
			})
		}
	}
	return out, missing
}

// properNounRe finds capitalized-word candidates (with an optional
// hyphenated suffix) in running text. It only works for cased scripts, which is
// why it is documented as a Spanish-only heuristic in DECISIONS.md.
var properNounRe = regexp.MustCompile(`[A-ZÁÉÍÓÚÑ][\p{Ll}áéíóúñ]{2,}(?:-[A-ZÁÉÍÓÚÑ]?[\p{Ll}áéíóúñ]+)?`)

// placeStopwords are common capitalized words (sentence starts, deity
// titles, pronouns...) that the proper-noun regex catches but are not place
// candidates worth cross-referencing.
var placeStopwords = map[string]bool{}

func init() {
	for _, w := range []string{
		"Dios", "Jehová", "Jehova", "Biblia", "Palabra", "Jesús", "Jesus", "Cristo",
		"Señor", "Señora", "Traducción", "Nuevo", "Mundo", "Rey", "Reina", "Él", "Su",
		"Este", "Esta", "Estos", "Estas", "Ese", "Esa", "Esos", "Esas", "Aunque",
		"Cuando", "Como", "Pero", "Además", "También", "Por", "Para", "Así", "Una",
		"Uno", "El", "La", "Los", "Las", "En", "De", "Del", "Sin", "Con", "Todo",
		"Toda", "Todos", "Todas", "Más", "Muy", "Solo", "Sólo", "Entre", "Antes",
		"Después", "Luego", "Otro", "Otra", "Según", "Nota", "Ver", "Véase",
	} {
		placeStopwords[w] = true
	}
}

// chapterPlaces is a best-effort, purely local heuristic (documented in the
// command help): it does not attempt real place/person NLP classification.
// It pulls capitalized-word candidates out of the chapter's study notes and
// footnotes (skipping each segment's first word, almost always a sentence
// start rather than a proper noun) and cross-references them against
// Perspicacia (it) by exact document title, which is the closest thing this
// library has to a place/term dictionary.
func (a *app) chapterPlaces(st *store.Store, verses []store.Verse) []placeOut {
	var texts []string
	for _, v := range verses {
		for _, n := range v.Notes {
			texts = append(texts, n.Text)
		}
		for _, f := range v.Footnotes {
			texts = append(texts, f.Text)
		}
	}
	names := extractPlaceCandidates(texts)
	out := make([]placeOut, 0, len(names))
	for _, name := range names {
		p := placeOut{Name: name}
		if d, err := st.DocByTitle("it", name); err == nil && d != nil {
			p.InIt, p.ItDocID = true, d.DocID
		}
		out = append(out, p)
	}
	return out
}

// extractPlaceCandidates is the pure part of chapterPlaces, kept separate so
// the heuristic can be unit tested without a database.
func extractPlaceCandidates(texts []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, text := range texts {
		for _, m := range properNounRe.FindAllStringIndex(text, -1) {
			if m[0] == 0 {
				continue // sentence/segment start: usually not a proper noun
			}
			name := text[m[0]:m[1]]
			if placeStopwords[name] || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func dedupStrings(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// renderChapterDossier writes the chapter Markdown, shrinking citation
// extracts (never dropping verses or citations) until the tokenEstimate
// heuristic fits targetChapterTokens or the shrink steps run out. The first
// step (0 = no cap) is what most chapters render at: citation paragraphs
// are normally short enough on their own.
func renderChapterDossier(d *chapterDossier) string {
	var body string
	for i, step := range extractCapSteps {
		body = renderChapterMD(d, step)
		d.TokenEst = tokenEstimate(body)
		if d.TokenEst <= targetChapterTokens || i == len(extractCapSteps)-1 {
			break
		}
	}
	return fmt.Sprintf("%s\n---\nEstimated tokens in this file: ~%d (heuristic: chars/4)\n", body, d.TokenEst)
}

// citationsExceed reports whether any citation extract is longer than limit
// runes (limit<=0 means unlimited: nothing is ever trimmed).
func citationsExceed(cites []citationOut, limit int) bool {
	if limit <= 0 {
		return false
	}
	for _, c := range cites {
		if len([]rune(c.Extract)) > limit {
			return true
		}
	}
	return false
}

func renderChapterMD(d *chapterDossier, extractCap int) string {
	d.ExtractsCut = citationsExceed(d.Citations, extractCap)

	var b strings.Builder
	fmt.Fprintf(&b, "# Dossier: %s\n\n", d.Ref)

	b.WriteString("## Index\n\n")
	fmt.Fprintf(&b, "- Verses: %d\n", d.VerseCount)
	fmt.Fprintf(&b, "- Citations found: %d\n", d.CiteCount)
	fmt.Fprintf(&b, "- Images found: %d\n", d.ImageCount)
	fmt.Fprintf(&b, "- Candidate places: %d\n", d.PlaceCount)
	if d.ExtractsCut {
		b.WriteString("- Citation extracts trimmed to fit the token budget\n")
	}
	b.WriteString("- NOT in the library:")
	if len(d.NotInLib) == 0 {
		b.WriteString(" (nothing)\n")
	} else {
		b.WriteString("\n")
		for _, m := range d.NotInLib {
			fmt.Fprintf(&b, "  - %s\n", m)
		}
	}

	b.WriteString("\n## Bible text\n\n")
	for _, v := range d.Verses {
		if v.Verse == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", verseLabel(v), v.Text)
	}

	var notes, fns, xrefs []string
	for _, v := range d.Verses {
		cv := fmt.Sprintf("%d:%d", v.Chapter, v.Verse)
		for _, n := range v.Notes {
			notes = append(notes, fmt.Sprintf("- %s %s: %s", cv, n.Label, n.Text))
		}
		for _, f := range v.Footnotes {
			fns = append(fns, fmt.Sprintf("- %s %s «%s»: %s", cv, f.Marker, f.Anchor, f.Text))
		}
		for _, x := range v.XRefs {
			xrefs = append(xrefs, fmt.Sprintf("- %s %s «%s» → %s", cv, x.Marker, x.Anchor, strings.Join(x.Refs, "; ")))
		}
	}
	section := func(title string, lines []string) {
		b.WriteString("\n## " + title + "\n\n")
		if len(lines) == 0 {
			b.WriteString("(none)\n")
			return
		}
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
	}
	section("Notas de estudio", notes)
	section("Notas al pie", fns)
	section("Referencias marginales", xrefs)

	b.WriteString("\n## Citations in the library (most relevant first)\n\n")
	if len(d.Citations) == 0 {
		b.WriteString("(none)\n")
	}
	for i, c := range d.Citations {
		var vs []string
		for _, v := range c.Verses {
			vs = append(vs, fmt.Sprint(v))
		}
		fmt.Fprintf(&b, "%d. docid %d · %s · %s · verses %s", i+1, c.DocID, c.Pub, c.Title, strings.Join(vs, ", "))
		if c.Year > 0 {
			fmt.Fprintf(&b, " · %d", c.Year)
		}
		b.WriteString("\n")
		if c.NoExtract {
			b.WriteString("   (no indexed extract)\n")
			continue
		}
		extract := c.Extract
		cut := false
		if r := []rune(extract); extractCap > 0 && len(r) > extractCap {
			extract = string(r[:extractCap]) + "…"
			cut = true
		}
		fmt.Fprintf(&b, "   > %s\n", extract)
		if cut {
			b.WriteString("   (extract trimmed; the whole text: pubkit doc " + fmt.Sprint(c.DocID) + ")\n")
		}
	}

	b.WriteString("\n## Images of the citing documents\n\n")
	if len(d.Images) == 0 {
		b.WriteString("(none)\n")
	}
	for _, im := range d.Images {
		fmt.Fprintf(&b, "- docid %d image %d: %s · %d×%d", im.DocID, im.Num, im.File, im.Width, im.Height)
		if im.Caption != "" {
			fmt.Fprintf(&b, " · %s", im.Caption)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## Places mentioned (candidates)\n\n")
	b.WriteString("A local heuristic: proper nouns from the study notes and footnotes, cross-referenced by exact title against the encyclopedic publication (it). It is not real geographic classification, so check them.\n\n")
	if len(d.Places) == 0 {
		b.WriteString("(none)\n")
	}
	for _, p := range d.Places {
		if p.InIt {
			fmt.Fprintf(&b, "- %s → it docid %d\n", p.Name, p.ItDocID)
		} else {
			fmt.Fprintf(&b, "- %s (not found in it)\n", p.Name)
		}
	}

	return b.String()
}
