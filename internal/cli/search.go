package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/bible"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

func (a *app) searchCmd() *cobra.Command {
	var pubs string
	var limit int
	var withBible, allWords, withVideos bool
	cmd := &cobra.Command{
		Use:     `search "<query>"`,
		Aliases: []string{"buscar", "s"},
		Short:   "Full-text search (FTS5) across the library",
		Long: `Searches the decrypted text of every synced publication.

A question is taken as evidence rather than as a requirement: the words that
narrow things down are searched for, a paragraph matching most of them counts, and
prose is ranked above index entries and covers. Which words narrow anything down is
measured against the library — a term in a fifth of all paragraphs cannot — so no
list of stopwords per language is carried or needed.

--videos searches the transcripts of the videos you have fetched as well, so one
question can be answered from the publications and from what was said on screen.

--all-words restores the stricter search, where every word must land in the same
paragraph. Accents are ignored either way; "in quotes" matches the exact phrase and
word* matches by prefix.

Every hit carries the citation in the form publications use for themselves, a link
that opens the paragraph in your language, and what kind of document it is.`,
		Example: `  pubkit search "how do I comfort someone who lost a loved one?"
  pubkit search "cistern" --pub it,w
  pubkit search "\"accurate knowledge\"" --all-words
  pubkit search "mire cistern" --bible`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			var filter []string
			for _, p := range strings.Split(pubs, ",") {
				if p = strings.TrimSpace(p); p != "" {
					filter = append(filter, p)
				}
			}
			search := st.Question
			if allWords {
				search = st.Search
			}
			hits, err := search(args[0], filter, limit)
			if err != nil {
				return err
			}
			var cues []store.CueHit
			if withVideos {
				if cues, err = st.SearchCues(args[0], a.lang, limit); err != nil {
					return err
				}
			}
			var verses []store.VerseHit
			if withBible {
				if verses, err = st.SearchVerses(args[0], limit); err != nil {
					return err
				}
			}
			if a.jsonOut {
				if hits == nil {
					hits = []store.SearchHit{}
				}
				if verses == nil {
					verses = []store.VerseHit{}
				}
				type videoHit struct {
					store.CueHit
					Timestamp string `json:"timestamp"`
					URL       string `json:"url"`
				}
				vids := make([]videoHit, 0, len(cues))
				for _, c := range cues {
					vids = append(vids, videoHit{CueHit: c, Timestamp: subs.FormatTS(c.Start),
						URL: subs.WatchURL(c.Key, c.Lang, c.Seconds)})
				}
				return a.printJSON(map[string]any{"query": args[0], "fts": store.FTSQuery(args[0]),
					"documents": hits, "verses": verses, "videos": vids})
			}
			if len(hits) == 0 && len(verses) == 0 && len(cues) == 0 {
				pl, _ := st.Pubs()
				if len(pl) == 0 {
					return errors.New("the library is empty: sync something first (pubkit sync it w nwtsty)")
				}
				a.printf("Nothing found for %q in %d publications.\n", args[0], len(pl))
				return nil
			}
			for _, h := range hits {
				a.printf("%-22s %s", h.Cite, h.Title)
				if h.Kind != "" && h.Kind != "article" {
					a.printf(" [%s]", h.Kind)
				}
				if h.Matches > 1 {
					a.printf(" (%d paragraphs match)", h.Matches)
				}
				a.printf("\n%-22s %s\n", "", h.Snippet)
				a.printf("%-22s %s\n", "", h.URL)
			}
			if len(cues) > 0 {
				a.printf("\nIn videos:\n")
				for _, c := range cues {
					a.printf("  %-28s %7s  %s\n", c.Key, subs.FormatTS(c.Start), c.Title)
					a.printf("  %38s%s\n", "", c.Snippet)
				}
			}
			if len(verses) > 0 {
				a.printf("\nBible verses:\n")
				for _, v := range verses {
					bk, _ := bible.BookByNum(v.Book)
					a.printf("  %s %d:%d  %s\n", bk.Short, v.Chapter, v.Verse, v.Snippet)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pubs, "pub", "", "limit to publications (comma-separated symbols: it,w,w13,nwtsty,mwb)")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of documents")
	cmd.Flags().BoolVar(&withBible, "bible", false, "search the Bible text as well")
	cmd.Flags().BoolVar(&withVideos, "videos", false, "search the transcripts of fetched videos as well")
	cmd.Flags().BoolVar(&allWords, "all-words", false, "require every word in the same paragraph, as this command used to")
	return cmd
}
