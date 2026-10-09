package cli

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/backup"
)

func (a *app) backupCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "backup", Aliases: []string{"respaldo"}, Short: "Inspect, merge and annotate JW Library backups locally"}
	inspect := &cobra.Command{Use: "inspect <backup.jwlibrary>", Aliases: []string{"inspeccionar"}, Short: "Validate the archive and report counts without private text", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		b, err := backup.Open(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		defer b.Close()
		i, err := b.Inspect(cmd.Context())
		if err != nil {
			return err
		}
		return a.printJSON(i)
	}}
	var output, prefer, device, report, base string
	var dry bool
	var interactive bool
	tables := map[string]*string{}
	merge := &cobra.Command{Use: "merge <backup> <backup> [backup...]", Aliases: []string{"unir"}, Short: "Merge two or more backups with configurable conflict priority", Long: `Merge schema 16 backups, remapping every relationship and preserving media files.
The first input wins by default. --prefer accepts an input path, newest or oldest.
Newest/oldest use the archive's lastModifiedDate; ties preserve input order.
Per-table preferences override the global preference. Overlapping highlights retain
the preferred range and only import uncovered numeric tokens. A whole-block range
with unknown bounds loses to an overlapping preferred range and is reported.
With --base, merge exactly two descendants and propagate deletions.
Deletion versus editing is a conflict resolved by preference or --interactive.
The output is a new archive; existing files are never overwritten.
--dry-run performs the same merge and validation without writing an archive.
The JSON report contains identifiers and counts, never note or answer text.`, Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !dry && output == "" {
			return errors.New("--output is required unless --dry-run is set")
		}
		prefs := map[string]string{}
		for t, p := range tables {
			prefs[t] = *p
		}
		opts := backup.MergeOptions{Base: base, Prefer: prefer, TablePrefer: prefs, Device: device, DryRun: dry}
		if interactive {
			resolver, err := a.backupResolver(cmd)
			if err != nil {
				return err
			}
			opts.Resolve = resolver
		}
		r, err := backup.Merge(cmd.Context(), args, output, opts)
		if err != nil {
			return err
		}
		if report != "" {
			f, e := os.OpenFile(report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return e
			}
			defer f.Close()
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			if e = enc.Encode(r); e != nil {
				return e
			}
		}
		return a.printJSON(r)
	}}
	f := merge.Flags()
	f.StringVarP(&output, "output", "o", "", "new output archive")
	f.StringVar(&base, "base", "", "common ancestor for a three-way merge of exactly two backups")
	f.StringVar(&prefer, "prefer", "", "preferred input path, newest or oldest (default: first input)")
	f.StringVar(&device, "device-name", "", "output device name (default: preferred input device with merged suffix)")
	f.StringVar(&report, "report", "", "write a private JSON report to a new file")
	f.BoolVar(&dry, "dry-run", false, "merge and validate without writing an archive")
	f.BoolVar(&interactive, "interactive", false, "review conflicting notes and answers in a terminal")
	for flag, t := range map[string]string{"notes": "Note", "highlights": "UserMark", "input-fields": "InputField", "tags": "Tag", "bookmarks": "Bookmark", "playlists": "PlaylistItem", "tag-maps": "TagMap"} {
		p := new(string)
		tables[t] = p
		f.StringVar(p, "prefer-"+flag, "", "override conflict preference for "+flag)
	}
	cmd.AddCommand(inspect, merge, a.backupAnnotateCmd())
	return cmd
}

func (a *app) backupAnnotateCmd() *cobra.Command {
	var output, planPath, device string
	cmd := &cobra.Command{Use: "annotate <backup.jwlibrary>", Aliases: []string{"anotar"}, Short: "Experimentally add highlights, a note and textarea answers from a JSON plan", Long: `Reads publication HTML from the local library. The plan supplies highlights
(docid, pid, quote, color), an optional note (highlight index, title, content),
and answers (docid, text_tag, value). Token positions are experimental and require
visual validation in JW Library. Existing highlights and answers are never replaced.`, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if output == "" || planPath == "" {
			return errors.New("--output and --plan are required")
		}
		f, err := os.Open(planPath)
		if err != nil {
			return err
		}
		defer f.Close()
		var plan backup.AnnotationPlan
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err = dec.Decode(&plan); err != nil {
			return err
		}
		st, err := a.store()
		if err != nil {
			return err
		}
		r, err := backup.Annotate(cmd.Context(), args[0], output, st.DB, plan, device)
		if err != nil {
			return err
		}
		return a.printJSON(r)
	}}
	cmd.Flags().StringVarP(&output, "output", "o", "", "new output archive")
	cmd.Flags().StringVar(&planPath, "plan", "", "private annotation plan JSON")
	cmd.Flags().StringVar(&device, "device-name", "", "output device name")
	return cmd
}
