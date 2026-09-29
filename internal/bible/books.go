// Package bible knows the Spanish book names of the NWT (TNM), parses Bible
// references the way Spanish publications write them and maps verses to the
// BibleVerseId numbering every JWPUB shares.
package bible

import (
	"strings"
	"unicode"
)

// Book is a Bible book with its Spanish names.
type Book struct {
	Num    int
	Name   string // "Jeremías"
	Short  string // study-note abbreviation: "Jer", "1Co"
	Medium string // Watchtower abbreviation: "Jer.", "1 Cor."
}

// Books is indexed by book number minus one.
var Books = [66]Book{
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
// official forms. Keys are already folded (see fold).
var extraAliases = map[string]int{
	"gen": 1, "gn": 1, "ge": 1,
	"exo": 2, "ex": 2,
	"lv": 3,
	"nm": 4, "nu": 4,
	"deu": 5,
	"jc":  7,
	"rt":  8,
	"1s":  9, "2s": 10, "1r": 11, "2r": 12,
	"1cro": 13, "2cro": 14,
	"sal": 19, "salmo": 19, "sa": 19,
	"pro":    20,
	"ecles":  21,
	"cantar": 22, "cantares": 22, "cantardeloscantares": 22,
	"isa": 23,
	"ez":  26,
	"dn":  27,
	"jl":  29,
	"mc":  41,
	"lc":  42,
	"hc":  44,
	"ga":  48,
	"fil": 50,
	"1ts": 52, "2ts": 53,
	"stg": 59,
	"rev": 66, "revelacion": 66, "apo": 66,
}

var aliasIndex = buildAliases()

func buildAliases() map[string]int {
	m := map[string]int{}
	for _, b := range Books {
		for _, name := range []string{b.Name, b.Short, b.Medium} {
			m[fold(name)] = b.Num
		}
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

// LookupBook finds a book by any accepted name or abbreviation.
func LookupBook(name string) (Book, bool) {
	n, ok := aliasIndex[fold(name)]
	if !ok {
		return Book{}, false
	}
	return Books[n-1], true
}

// BookByNum returns the book with number n (1-66).
func BookByNum(n int) (Book, bool) {
	if n < 1 || n > 66 {
		return Book{}, false
	}
	return Books[n-1], true
}
