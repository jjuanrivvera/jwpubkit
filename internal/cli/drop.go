package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

// dropCmd removes one language's indexed library.
//
// It exists because doing it by hand is a trap, and the trap has been sprung:
// SQLite in WAL mode leaves the main database file a few kilobytes while the
// indexed content sits in the -wal beside it, so `ls -l` makes a full library
// look empty. Delete the .db alone and the orphaned -wal is applied to whatever
// database is created next. Counting publications and removing the three files
// together is exactly the kind of thing a command should do once, correctly,
// instead of every person doing it from memory.
func (a *app) dropCmd() *cobra.Command {
	var yes, withCache bool
	cmd := &cobra.Command{
		Use:   "drop",
		Short: "Remove the indexed library of one language",
		Long: `Removes the database holding the publications indexed for --language, listing what
it is about to destroy first.

The three files SQLite uses (the database, its -wal and its -shm) are removed
together: the content of a freshly indexed library lives in the -wal, so removing
the database alone both leaves the data and leaves the -wal to be replayed onto
the next database created in its place.

The downloaded .jwpub files are kept by default — they are shared between
languages and re-indexing from them needs no network. --cache removes the ones
belonging to this language too.`,
		Example: `  pubkit drop --language E
  pubkit drop --language E --cache --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			pubs, err := st.Pubs()
			if err != nil {
				return err
			}
			files := st.Files()
			size := st.Size()

			a.printf("language %s · %s\n", a.lang, st.Path)
			if len(pubs) == 0 {
				a.printf("  no publications indexed\n")
			} else {
				for _, p := range pubs {
					a.printf("  %-18s %5d docs  %s\n", p.Key, p.Docs, p.Title)
				}
			}
			a.printf("  %d publications, %s of index in %d files\n", len(pubs), mb(size), countExisting(files))

			var cached []string
			if withCache {
				cached = a.cachedFilesFor(pubs)
				for _, f := range cached {
					a.printf("  cache: %s\n", f)
				}
			}
			if !yes {
				a.printf("\nNothing was removed. Run it again with --yes to remove the above.\n")
				return nil
			}
			// The handle has to go before the files, or the -wal is rewritten on close.
			if err := st.Close(); err != nil {
				return err
			}
			a.st = nil
			for _, f := range append(files, cached...) {
				if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("removing %s: %w", f, err)
				}
			}
			a.printf("\nremoved %s of index%s\n", mb(size), plural(len(cached), " and %d cached publication"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "actually remove it; without this the command only reports")
	cmd.Flags().BoolVar(&withCache, "cache", false, "also remove this language's downloaded .jwpub files")
	return cmd
}

func countExisting(paths []string) int {
	n := 0
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			n++
		}
	}
	return n
}

// cachedFilesFor lists the downloaded publications of the language being
// dropped. Their names carry the language, which is why one cache can serve all
// of them.
func (a *app) cachedFilesFor(pubs []store.Pub) []string {
	var out []string
	for _, p := range pubs {
		if strings.TrimSpace(p.File) != "" {
			out = append(out, p.File)
		}
	}
	return out
}

func plural(n int, format string) string {
	if n == 0 {
		return ""
	}
	s := fmt.Sprintf(format, n)
	if n != 1 {
		s += "s"
	}
	return s
}
