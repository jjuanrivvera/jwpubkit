package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
)

type mediaCatalog struct {
	Roots      []string        `json:"roots"`
	Language   string          `json:"language"`
	Categories []cdn.Category  `json:"categories"`
	Media      []cdn.MediaItem `json:"media"`
}

func (a *app) catalogPath() string {
	return filepath.Join(a.libDir, "catalog", "media."+a.lang+".json")
}
func (a *app) readCatalog() (mediaCatalog, error) {
	var cat mediaCatalog
	b, err := os.ReadFile(a.catalogPath())
	if err != nil {
		return cat, fmt.Errorf("read catalog (run pubkit catalog media --refresh): %w", err)
	}
	err = json.Unmarshal(b, &cat)
	if err == nil && cat.Language != a.lang {
		err = fmt.Errorf("catalog language mismatch")
	}
	return cat, err
}
func (a *app) catalogCmd() *cobra.Command {
	root := &cobra.Command{Use: "catalog", Short: "Discover publication and audiovisual metadata"}
	var refresh, resume bool
	var file string
	var interval time.Duration
	cmd := &cobra.Command{Use: "media", Short: "Traverse all mediator categories and deduplicate media", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		a.ctx = cmd.Context()
		var cat mediaCatalog
		var err error
		switch {
		case file != "":
			b, e := os.ReadFile(file)
			if e != nil {
				return e
			}
			if err = json.Unmarshal(b, &cat); err != nil {
				return err
			}
			if cat.Language != a.lang {
				return fmt.Errorf("catalog language mismatch")
			}
		case refresh:
			if a.offline {
				return errOffline
			}
			cat, err = a.crawlMedia(interval, resume)
			if err != nil {
				return err
			}
		default:
			cat, err = a.readCatalog()
			if err != nil {
				return err
			}
		}
		for _, m := range cat.Media {
			if !cdn.ValidMediaKey(m.LanguageAgnosticNaturalKey) {
				return fmt.Errorf("invalid catalog media key %q", m.LanguageAgnosticNaturalKey)
			}
		}
		if refresh || file != "" {
			if err := os.MkdirAll(filepath.Dir(a.catalogPath()), 0o755); err != nil {
				return err
			}
			b, err := json.Marshal(cat)
			if err != nil {
				return err
			}
			temp := a.catalogPath() + ".part"
			if err := os.WriteFile(temp, b, 0o644); err != nil {
				return err
			}
			if err := os.Rename(temp, a.catalogPath()); err != nil {
				return err
			}
		}
		st, err := a.commandStore(cmd)
		if err != nil {
			return err
		}
		if err := st.PutCatalog(cat.Media, a.lang); err != nil {
			return err
		}
		coverage, err := st.CatalogCoverage(a.lang)
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(map[string]any{"catalog": cat, "coverage": coverage})
		}
		counts := map[string]int{}
		for _, m := range cat.Media {
			counts[m.Type]++
		}
		a.printf("%d categories, %d unique media (%v)\nCoverage: %v\nCache: %s\n", len(cat.Categories), len(cat.Media), counts, coverage, a.catalogPath())
		return nil
	}}
	cmd.Flags().BoolVar(&resume, "resume", false, "reuse cached category responses from an interrupted refresh")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "refresh all categories from the CDN")
	cmd.Flags().StringVar(&file, "file", "", "import a previously downloaded media catalog JSON")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "pause between category requests")
	root.AddCommand(cmd, a.publicationCatalogCmd())
	return root
}
func (a *app) crawlMedia(interval time.Duration, resume bool) (mediaCatalog, error) {
	cat := mediaCatalog{Language: a.lang, Categories: []cdn.Category{}, Media: []cdn.MediaItem{}}
	if interval < 0 {
		return cat, fmt.Errorf("interval must not be negative")
	}
	root, err := a.client().Categories(a.ctx, "")
	if err != nil {
		return cat, err
	}
	for _, node := range root.Subcategories {
		cat.Roots = append(cat.Roots, node.Key)
	}
	queue := append([]cdn.Category(nil), root.Subcategories...)
	seen := map[string]bool{}
	items := map[string]*cdn.MediaItem{}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.Key == "" || seen[node.Key] {
			continue
		}
		seen[node.Key] = true

		if filepath.Base(node.Key) != node.Key || strings.ContainsAny(node.Key, `\:`) {
			return cat, fmt.Errorf("invalid category key %q", node.Key)
		}
		cache := filepath.Join(a.libDir, "catalog", "categories."+a.lang, node.Key+".json")
		var detail cdn.Category
		cached := false
		if resume {
			if b, e := os.ReadFile(cache); e == nil && json.Unmarshal(b, &detail) == nil {
				cached = true
			}
		}
		if !cached {
			if err := cdn.Pause(a.ctx, interval); err != nil {
				return cat, err
			}
			fetched, err := a.client().Categories(a.ctx, node.Key)
			if err != nil {
				return cat, fmt.Errorf("category %s: %w", node.Key, err)
			}
			detail = *fetched
			b, err := json.Marshal(detail)
			if err != nil {
				return cat, err
			}
			if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
				return cat, err
			}
			if err := os.WriteFile(cache+".part", b, 0o644); err != nil {
				return cat, err
			}
			if err := os.Rename(cache+".part", cache); err != nil {
				return cat, err
			}
		}
		entry := cdn.Category{Key: node.Key, Name: detail.Name, Parent: node.Parent, Subcategories: detail.Subcategories}
		for _, child := range detail.Subcategories {
			entry.Children = append(entry.Children, child.Key)
			child.Parent = node.Key
			queue = append(queue, child)
		}
		cat.Categories = append(cat.Categories, entry)

		for _, m := range detail.Media {
			key := m.LanguageAgnosticNaturalKey
			if key == "" {
				return cat, fmt.Errorf("category %s has an item without a key", node.Key)
			}
			if old := items[key]; old != nil {
				if !contains(old.FoundIn, node.Key) {
					old.FoundIn = append(old.FoundIn, node.Key)
				}
				continue
			}
			m.FoundIn = []string{node.Key}
			items[key] = &m
		}
		a.logf("catalog: %d categories, %d media", len(cat.Categories), len(items))
	}
	for _, m := range items {
		cat.Media = append(cat.Media, *m)
	}
	sort.Slice(cat.Media, func(i, j int) bool {
		return cat.Media[i].LanguageAgnosticNaturalKey < cat.Media[j].LanguageAgnosticNaturalKey
	})
	return cat, nil
}
