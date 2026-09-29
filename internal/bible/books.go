// Package bible names the books of the Bible, parses references the way
// publications write them and maps verses to the BibleVerseId numbering every
// JWPUB shares.
//
// Names are per language. English and Spanish ship built in; any other language
// is registered at run time from the Bible in the library (Register), because a
// tool that reads publications in every language jw.org publishes has no business
// carrying a hand-written table for each of them.
package bible

import (
	"strings"
	"sync"
	"unicode"
)

// Book is a Bible book with the three name forms publications use.
type Book struct {
	Num    int
	Name   string // full name: "Jeremiah"
	Short  string // study-note abbreviation: "Jer", "1Co"
	Medium string // article abbreviation: "Jer.", "1 Cor."
}

// DefaultLang is the language whose names are used when none is chosen.
const DefaultLang = "E"

var booksEN = [66]Book{
	{1, "Genesis", "Ge", "Gen."},
	{2, "Exodus", "Ex", "Ex."},
	{3, "Leviticus", "Le", "Lev."},
	{4, "Numbers", "Nu", "Num."},
	{5, "Deuteronomy", "De", "Deut."},
	{6, "Joshua", "Jos", "Josh."},
	{7, "Judges", "Jg", "Judg."},
	{8, "Ruth", "Ru", "Ruth"},
	{9, "1 Samuel", "1Sa", "1 Sam."},
	{10, "2 Samuel", "2Sa", "2 Sam."},
	{11, "1 Kings", "1Ki", "1 Ki."},
	{12, "2 Kings", "2Ki", "2 Ki."},
	{13, "1 Chronicles", "1Ch", "1 Chron."},
	{14, "2 Chronicles", "2Ch", "2 Chron."},
	{15, "Ezra", "Ezr", "Ezra"},
	{16, "Nehemiah", "Ne", "Neh."},
	{17, "Esther", "Es", "Esther"},
	{18, "Job", "Job", "Job"},
	{19, "Psalms", "Ps", "Ps."},
	{20, "Proverbs", "Pr", "Prov."},
	{21, "Ecclesiastes", "Ec", "Eccl."},
	{22, "Song of Solomon", "Ca", "Song of Sol."},
	{23, "Isaiah", "Isa", "Isa."},
	{24, "Jeremiah", "Jer", "Jer."},
	{25, "Lamentations", "La", "Lam."},
	{26, "Ezekiel", "Eze", "Ezek."},
	{27, "Daniel", "Da", "Dan."},
	{28, "Hosea", "Ho", "Hos."},
	{29, "Joel", "Joe", "Joel"},
	{30, "Amos", "Am", "Amos"},
	{31, "Obadiah", "Ob", "Obad."},
	{32, "Jonah", "Jon", "Jonah"},
	{33, "Micah", "Mic", "Mic."},
	{34, "Nahum", "Na", "Nah."},
	{35, "Habakkuk", "Hab", "Hab."},
	{36, "Zephaniah", "Zep", "Zeph."},
	{37, "Haggai", "Hag", "Hag."},
	{38, "Zechariah", "Zec", "Zech."},
	{39, "Malachi", "Mal", "Mal."},
	{40, "Matthew", "Mt", "Matt."},
	{41, "Mark", "Mr", "Mark"},
	{42, "Luke", "Lu", "Luke"},
	{43, "John", "Joh", "John"},
	{44, "Acts", "Ac", "Acts"},
	{45, "Romans", "Ro", "Rom."},
	{46, "1 Corinthians", "1Co", "1 Cor."},
	{47, "2 Corinthians", "2Co", "2 Cor."},
	{48, "Galatians", "Ga", "Gal."},
	{49, "Ephesians", "Eph", "Eph."},
	{50, "Philippians", "Php", "Phil."},
	{51, "Colossians", "Col", "Col."},
	{52, "1 Thessalonians", "1Th", "1 Thess."},
	{53, "2 Thessalonians", "2Th", "2 Thess."},
	{54, "1 Timothy", "1Ti", "1 Tim."},
	{55, "2 Timothy", "2Ti", "2 Tim."},
	{56, "Titus", "Tit", "Titus"},
	{57, "Philemon", "Phm", "Philem."},
	{58, "Hebrews", "Heb", "Heb."},
	{59, "James", "Jas", "Jas."},
	{60, "1 Peter", "1Pe", "1 Pet."},
	{61, "2 Peter", "2Pe", "2 Pet."},
	{62, "1 John", "1Jo", "1 John"},
	{63, "2 John", "2Jo", "2 John"},
	{64, "3 John", "3Jo", "3 John"},
	{65, "Jude", "Jude", "Jude"},
	{66, "Revelation", "Re", "Rev."},
}

var booksES = [66]Book{
	{1, "Génesis", "Gé", "Gén."},
	{2, "Éxodo", "Éx", "Éx."},
	{3, "Levítico", "Le", "Lev."},
	{4, "Números", "Nú", "Núm."},
	{5, "Deuteronomio", "Dt", "Deut."},
	{6, "Josué", "Jos", "Jos."},
	{7, "Jueces", "Jue", "Juec."},
	{8, "Rut", "Rut", "Rut"},
	{9, "1 Samuel", "1Sa", "1 Sam."},
	{10, "2 Samuel", "2Sa", "2 Sam."},
	{11, "1 Reyes", "1Re", "1 Rey."},
	{12, "2 Reyes", "2Re", "2 Rey."},
	{13, "1 Crónicas", "1Cr", "1 Crón."},
	{14, "2 Crónicas", "2Cr", "2 Crón."},
	{15, "Esdras", "Esd", "Esd."},
	{16, "Nehemías", "Ne", "Neh."},
	{17, "Ester", "Est", "Est."},
	{18, "Job", "Job", "Job"},
	{19, "Salmos", "Sl", "Sal."},
	{20, "Proverbios", "Pr", "Prov."},
	{21, "Eclesiastés", "Ec", "Ecl."},
	{22, "El Cantar de los Cantares", "Can", "Cant."},
	{23, "Isaías", "Is", "Is."},
	{24, "Jeremías", "Jer", "Jer."},
	{25, "Lamentaciones", "Lam", "Lam."},
	{26, "Ezequiel", "Eze", "Ezeq."},
	{27, "Daniel", "Da", "Dan."},
	{28, "Oseas", "Os", "Os."},
	{29, "Joel", "Joel", "Joel"},
	{30, "Amós", "Am", "Amós"},
	{31, "Abdías", "Abd", "Abd."},
	{32, "Jonás", "Jon", "Jon."},
	{33, "Miqueas", "Miq", "Miq."},
	{34, "Nahúm", "Nah", "Nah."},
	{35, "Habacuc", "Hab", "Hab."},
	{36, "Sofonías", "Sof", "Sof."},
	{37, "Ageo", "Ag", "Ageo"},
	{38, "Zacarías", "Zac", "Zac."},
	{39, "Malaquías", "Mal", "Mal."},
	{40, "Mateo", "Mt", "Mat."},
	{41, "Marcos", "Mr", "Mar."},
	{42, "Lucas", "Lu", "Luc."},
	{43, "Juan", "Jn", "Juan"},
	{44, "Hechos", "Hch", "Hech."},
	{45, "Romanos", "Ro", "Rom."},
	{46, "1 Corintios", "1Co", "1 Cor."},
	{47, "2 Corintios", "2Co", "2 Cor."},
	{48, "Gálatas", "Gál", "Gál."},
	{49, "Efesios", "Ef", "Efes."},
	{50, "Filipenses", "Flp", "Filip."},
	{51, "Colosenses", "Col", "Col."},
	{52, "1 Tesalonicenses", "1Te", "1 Tes."},
	{53, "2 Tesalonicenses", "2Te", "2 Tes."},
	{54, "1 Timoteo", "1Ti", "1 Tim."},
	{55, "2 Timoteo", "2Ti", "2 Tim."},
	{56, "Tito", "Tit", "Tito"},
	{57, "Filemón", "Flm", "Filem."},
	{58, "Hebreos", "Heb", "Heb."},
	{59, "Santiago", "Snt", "Sant."},
	{60, "1 Pedro", "1Pe", "1 Ped."},
	{61, "2 Pedro", "2Pe", "2 Ped."},
	{62, "1 Juan", "1Jn", "1 Juan"},
	{63, "2 Juan", "2Jn", "2 Juan"},
	{64, "3 Juan", "3Jn", "3 Juan"},
	{65, "Judas", "Jud", "Jud."},
	{66, "Apocalipsis", "Ap", "Apoc."},
}

// extraAliases covers spellings people type that are not one of the three
// official forms of a built-in language. Keys are already folded (see fold).
var extraAliases = map[string]int{
	// English
	"gen": 1, "exo": 2, "lev": 3, "num": 4, "deut": 5, "josh": 6, "judg": 7,
	"1sam": 9, "2sam": 10, "1kings": 11, "2kings": 12, "1chron": 13, "2chron": 14,
	"neh": 16, "esth": 17, "psalm": 19, "prov": 20, "eccl": 21, "song": 22,
	"songofsongs": 22, "lam": 25, "ezek": 26, "dan": 27, "hos": 28, "obad": 31,
	"zeph": 36, "zech": 38, "matt": 40, "mark": 41, "luke": 42, "john": 43,
	"acts": 44, "rom": 45, "1cor": 46, "2cor": 47, "gal": 48, "phil": 50,
	"1thess": 52, "2thess": 53, "1tim": 54, "2tim": 55, "philem": 57,
	"james": 59, "1pet": 60, "2pet": 61, "1john": 62, "2john": 63, "3john": 64,
	"rev": 66, "revelation": 66, "apocalypse": 66,
	// Spanish
	"gn": 1, "ge": 1, "ex": 2, "lv": 3, "nm": 4, "nu": 4, "deu": 5, "jc": 7,
	"rt": 8, "1s": 9, "2s": 10, "1r": 11, "2r": 12, "1cro": 13, "2cro": 14,
	"sal": 19, "salmo": 19, "sa": 19, "pro": 20, "ecles": 21, "cantar": 22,
	"cantares": 22, "cantardeloscantares": 22, "isa": 23, "ez": 26, "dn": 27,
	"jl": 29, "mc": 41, "lc": 42, "hc": 44, "ga": 48, "fil": 50, "1ts": 52,
	"2ts": 53, "stg": 59, "apo": 66, "revelacion": 66,
}

var (
	mu     sync.RWMutex
	tables = map[string]*[66]Book{DefaultLang: &booksEN, "S": &booksES}
	// builtIn are the languages whose canonical citation forms this package
	// already knows; what a library teaches about them is added as an alias and
	// never replaces them.
	builtIn = map[string]bool{DefaultLang: true, "S": true}
	// learned holds the display titles read from indexed Bibles, so a long form
	// like a full gospel title still resolves without becoming how references
	// are printed.
	learned = map[string]map[int]string{}
	active  = DefaultLang
	aliases = buildAliases()
)

// Register teaches the package a language's book names, as read from a Bible in
// the library. The names always become accepted spellings; for a language with
// no built-in table they also become how its books are printed, which is what
// makes every language jw.org publishes usable without shipping a table for each.
func Register(lang string, names map[int]string) {
	if lang == "" || len(names) == 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if learned[lang] == nil {
		learned[lang] = map[int]string{}
	}
	for num, name := range names {
		if num < 1 || num > 66 || strings.TrimSpace(name) == "" {
			continue
		}
		learned[lang][num] = name
	}
	if !builtIn[lang] {
		t, ok := tables[lang]
		if !ok {
			var empty [66]Book
			for i := range empty {
				empty[i] = Book{Num: i + 1}
			}
			t = &empty
			tables[lang] = t
		}
		for num, name := range learned[lang] {
			b := &t[num-1]
			b.Num, b.Name = num, name
			if b.Short == "" {
				b.Short = name
			}
			if b.Medium == "" {
				b.Medium = name
			}
		}
	}
	aliases = buildAliasesLocked()
}

// UseLanguage picks the names used for display. Parsing is unaffected: a
// reference typed in any known language is still understood, which is what a
// user switching languages mid-session actually wants.
func UseLanguage(lang string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := tables[lang]; ok {
		active = lang
		return
	}
	active = DefaultLang
}

// Languages lists the languages whose book names are known.
func Languages() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(tables))
	for l := range tables {
		out = append(out, l)
	}
	return out
}

// books returns the table in use for display.
func books() *[66]Book {
	mu.RLock()
	defer mu.RUnlock()
	if t, ok := tables[active]; ok {
		return t
	}
	return &booksEN
}

func buildAliases() map[string]int {
	mu.RLock()
	defer mu.RUnlock()
	return buildAliasesLocked()
}

// buildAliasesLocked indexes every name of every known language. Across
// languages the forms rarely collide, and when they do they mean the same book;
// the active language is indexed last so it wins if they ever do not.
func buildAliasesLocked() map[string]int {
	m := map[string]int{}
	index := func(t *[66]Book) {
		for _, b := range t {
			for _, name := range []string{b.Name, b.Short, b.Medium} {
				if name != "" {
					m[fold(name)] = b.Num
				}
			}
		}
	}
	for lang, t := range tables {
		if lang != active {
			index(t)
		}
	}
	for _, names := range learned {
		for num, name := range names {
			if _, taken := m[fold(name)]; !taken {
				m[fold(name)] = num
			}
		}
	}
	if t, ok := tables[active]; ok {
		index(t)
	}
	m[fold("Cantar de los Cantares")] = 22
	for k, v := range extraAliases {
		m[k] = v
	}
	return m
}

// fold lowercases, drops diacritics, dots and spaces: "1 Cor." → "1cor",
// "Éxodo" → "exodo".
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'á', 'à', 'ä', 'â':
			r = 'a'
		case 'é', 'è', 'ë', 'ê':
			r = 'e'
		case 'í', 'ì', 'ï', 'î':
			r = 'i'
		case 'ó', 'ò', 'ö', 'ô':
			r = 'o'
		case 'ú', 'ù', 'ü', 'û':
			r = 'u'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LookupBook finds a book by any accepted name or abbreviation, in any language
// the package knows.
func LookupBook(name string) (Book, bool) {
	mu.RLock()
	n, ok := aliases[fold(name)]
	mu.RUnlock()
	if !ok {
		return Book{}, false
	}
	return BookByNum(n)
}

// BookByNum returns book n (1-66) with the names of the language in use.
func BookByNum(n int) (Book, bool) {
	if n < 1 || n > 66 {
		return Book{}, false
	}
	b := books()[n-1]
	if b.Name == "" { // a partial table learned from a library
		return booksEN[n-1], true
	}
	return b, true
}
