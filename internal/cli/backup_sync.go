package cli

import (
	"errors"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/backup"
)

func (a *app) backupSyncCmd() *cobra.Command {
	var opts backup.SyncOptions
	var watch string
	var interactive bool
	tables := map[string]*string{}
	cmd := &cobra.Command{Use: "sync [incoming.jwlibrary...]", Aliases: []string{"sincronizar"}, Short: "Maintain a master backup with three-way synchronization and rotating history", Long: `Import incoming snapshots into the configured backup_store (override with --store).
The first import initializes the master; later imports merge against the stored base.
Each batch uses one ancestor. After success, master.jwlibrary and base.jwlibrary
contain the new master. Restore that master on devices before their next exports.
Use --base to supply the correct ancestor for older descendants.
--watch waits for stable .jwlibrary files and moves successful imports to procesados.
The store is private, locked against concurrent writers, and committed atomically.
Newest/oldest compare archive dates. --prefer master or incoming selects a side.
Existing incoming files are retained unless --watch moves them after successful import.`, Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && watch == "" {
			return errors.New("provide incoming backups or --watch <directory>")
		}
		if opts.HistoryLimit < 0 {
			return errors.New("--history-limit cannot be negative")
		}
		return nil
	}, ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"jwlibrary"}, cobra.ShellCompDirectiveFilterFileExt
	}, RunE: func(cmd *cobra.Command, args []string) error {
		opts.TablePrefer = map[string]string{}
		for t, p := range tables {
			opts.TablePrefer[t] = *p
		}
		if interactive {
			resolver, err := a.backupResolver(cmd)
			if err != nil {
				return err
			}
			opts.Resolve = resolver
		}
		dir := a.defaultBackupStore(cmd)
		if watch != "" {
			if _, err := backup.ValidateWatch(dir, watch); err != nil {
				return err
			}
		}
		if len(args) > 0 {
			r, err := backup.Sync(cmd.Context(), dir, args, opts)
			if err != nil {
				return err
			}
			if err = a.printJSON(r); err != nil {
				return err
			}
		}
		if watch != "" {
			return backup.Watch(cmd.Context(), dir, watch, opts, time.Second, func(event backup.WatchEvent) error { return a.printJSON(event) })
		}
		return nil
	}}
	f := cmd.Flags()
	f.StringVar(&watch, "watch", "", "watch an incoming directory and move imports to procesados")
	f.IntVar(&opts.HistoryLimit, "history-limit", 10, "number of previous masters to retain (0 keeps only current)")
	f.StringVar(&opts.Prefer, "prefer", "newest", "conflict priority: newest, oldest, master, incoming or an incoming path")
	f.StringVar(&opts.Base, "base", "", "override the stored common ancestor for older descendants")
	f.StringVar(&opts.Device, "device-name", "pubkit master", "output master device name")
	f.BoolVar(&interactive, "interactive", false, "review conflicting versions in a terminal")
	for flag, t := range map[string]string{"notes": "Note", "highlights": "UserMark", "input-fields": "InputField", "tags": "Tag", "bookmarks": "Bookmark", "playlists": "PlaylistItem", "tag-maps": "TagMap"} {
		p := new(string)
		tables[t] = p
		f.StringVar(p, "prefer-"+flag, "", "override conflict priority for "+flag)
	}
	_ = cmd.MarkFlagDirname("watch")
	_ = cmd.MarkFlagFilename("base", "jwlibrary")
	_ = cmd.RegisterFlagCompletionFunc("prefer", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"newest", "oldest", "master", "incoming"}, cobra.ShellCompDirectiveDefault
	})
	return cmd
}

func (a *app) backupStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"estado"}, Short: "Show the stored master, counts, history and last synchronization", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, err := backup.Status(cmd.Context(), a.defaultBackupStore(cmd))
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(s)
		}
		a.printf("Store: %s\n", s.Store)
		if !s.Initialized {
			a.printf("No master yet; import a backup with backup sync.\n")
			return nil
		}
		a.printf("Master: %s\nBase: %s\nMaster date: %s\nLast synchronization: %s\nDevice: %s\nPrevious masters: %d\n", s.Master, s.Base, s.MasterDate, s.LastSync, s.Device, len(s.History))
		tables := make([]string, 0, len(s.Counts))
		for table := range s.Counts {
			tables = append(tables, table)
		}
		sort.Strings(tables)
		for _, table := range tables {
			a.printf("%-32s %d\n", table, s.Counts[table])
		}
		return nil
	}}
}
