package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/config"
)

// configCmd answers "why is it behaving like that?" — the question this command
// exists for. A setting that comes from somewhere invisible is the hardest kind
// of bug to see: an exported shell variable that a cron job never reads makes
// the tool quietly do the wrong thing, and nothing in the output says so.
func (a *app) configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the settings in effect and where each one came from",
		Long: `Prints every setting, its value, and what decided it: a flag, an environment
variable, the settings file, or the built-in default.

The settings file is read from $JWPUBKIT_CONFIG, else $XDG_CONFIG_HOME/pubkit/config,
else ~/.config/pubkit/config. It takes one setting per line:

    # the library on this machine is in spanish
    language = S
    library = ~/.local/share/jwlib
    media_dir = ~/media
    backup_store = ~/backups

A flag beats an environment variable, which beats the file, which beats the default.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := a.cfg.Path
			if path == "" {
				path = config.Path()
			}
			rows := []struct{ name, value, from string }{
				{"language", a.lang, a.origin(cmd, "language")},
				{"library", a.libDir, a.origin(cmd, "library")},
				{"media_dir", a.defaultMediaDir(), a.mediaOrigin()},
				{"backup_store", a.defaultBackupStore(cmd), a.origin(cmd, "store")},
			}
			// Naming the database file matters: it is per language, and someone
			// reasoning about a file the tool never showed them is how a full
			// library got mistaken for an empty one.
			if st, err := a.store(); err == nil {
				rows = append(rows, struct{ name, value, from string }{
					"database", st.Path, "derived from library and language"})
			}
			if a.jsonOut {
				out := map[string]any{"config_file": path, "config_file_read": a.cfg.Path != ""}
				for _, r := range rows {
					out[r.name] = map[string]string{"value": r.value, "from": r.from}
				}
				return a.printJSON(out)
			}
			a.printf("settings file: %s", path)
			if a.cfg.Path == "" {
				a.printf("  (not present)")
			}
			a.printf("\n\n")
			for _, r := range rows {
				a.printf("%-10s %-46s %s\n", r.name, r.value, r.from)
			}
			return nil
		},
	}
}

// mediaOrigin explains the media directory the way the other settings are
// explained, since it is resolved at the point of use rather than at startup.
func (a *app) mediaOrigin() string {
	for _, env := range []string{"JWPUBKIT_MEDIA_DIR", "JWLIB_MEDIA_DIR"} {
		if v := envOf(env); v != "" {
			return env
		}
	}
	if a.cfg.MediaDir != "" {
		return a.cfg.Path
	}
	return "beside the library (" + filepath.Base(a.libDir) + "/media)"
}

func (a *app) defaultBackupStore(cmd *cobra.Command) string {
	if a.origin(cmd, "store") == "built-in default" {
		return filepath.Join(a.libDir, "backups")
	}
	return a.backupStore
}
