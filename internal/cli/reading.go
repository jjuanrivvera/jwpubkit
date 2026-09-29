package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/content"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

// readingTimeCmd estimates how long something takes to read aloud: a document,
// a Bible passage, or whatever arrives on standard input.
//
// It is an estimate and says so. The rates are an assumption about unhurried
// reading, not a measurement of anybody, and they are overridable — which is the
// honest way to ship a number somebody will plan a meeting part around.
func (a *app) readingTimeCmd() *cobra.Command {
	var wpm, cpm float64
	var reference string
	cmd := &cobra.Command{
		Use:     "reading-time [docid]",
		Aliases: []string{"read-time"},
		Short:   "Estimate how long a text takes to read aloud",
		Long: `Estimates reading-aloud time for a document from the library, for a Bible passage
(--reference), or for text piped in.

Words are counted where the script separates them and characters where it does
not, because counting words in Chinese or Thai would be off by an order of
magnitude. The rates are assumptions (130 words or 350 characters a minute) and
both can be overridden.`,
		Example: `  pubkit reading-time 1102025901
  pubkit reading-time --reference "Jer 38:1-13"
  echo "the text of a talk" | pubkit reading-time
  pubkit reading-time 1102025901 --wpm 150`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rate := content.ReadingRate{WordsPerMinute: wpm, CharsPerMinute: cpm}
			text, source, err := a.textToRead(args, reference)
			if err != nil {
				return err
			}
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("there is no text to read in %s", source)
			}
			d := content.ReadingTime(text, rate)
			words, dense := content.CountUnits(text)
			if a.jsonOut {
				return a.printJSON(map[string]any{
					"source": source, "seconds": int(d.Seconds()), "readable": subs.FormatTS(d),
					"words": words, "characters_in_dense_scripts": dense,
					"words_per_minute": rateOr(wpm, content.DefaultRate.WordsPerMinute),
					"chars_per_minute": rateOr(cpm, content.DefaultRate.CharsPerMinute),
					"estimate":         true,
				})
			}
			a.printf("%s\n", source)
			a.printf("  about %s read aloud (an estimate)\n", subs.FormatTS(d))
			if words > 0 {
				a.printf("  %d words\n", words)
			}
			if dense > 0 {
				a.printf("  %d characters in scripts written without word breaks\n", dense)
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&wpm, "wpm", 0, "words a minute for scripts that separate words (default 130)")
	cmd.Flags().Float64Var(&cpm, "cpm", 0, "characters a minute for scripts that do not (default 350)")
	cmd.Flags().StringVar(&reference, "reference", "", "estimate a Bible passage instead of a document")
	return cmd
}

// textToRead resolves what the user pointed at into plain text, and names it.
func (a *app) textToRead(args []string, reference string) (text, source string, err error) {
	switch {
	case reference != "":
		return a.verseText(reference)
	case len(args) == 1:
		docid, err := strconv.Atoi(args[0])
		if err != nil {
			return "", "", fmt.Errorf("invalid docid %q", args[0])
		}
		st, err := a.store()
		if err != nil {
			return "", "", err
		}
		_, parsed, err := a.loadDoc(st, docid)
		if err != nil {
			return "", "", err
		}
		return parsed.PlainText(), fmt.Sprintf("docid %d", docid), nil
	default:
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", "", err
		}
		return string(b), "standard input", nil
	}
}

func rateOr(given, fallback float64) float64 {
	if given > 0 {
		return given
	}
	return fallback
}
