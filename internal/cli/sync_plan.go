package cli

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/store"
)

type syncPlanRow struct {
	Key                string `json:"key"`
	Symbol             string `json:"symbol"`
	Issue              string `json:"issue"`
	Title              string `json:"title"`
	File               string `json:"file"`
	DownloadBytes      int64  `json:"download_bytes"`
	EstimatedDiskBytes int64  `json:"estimated_additional_disk_bytes"`
	Checksum           string `json:"checksum"`
	Cached             bool   `json:"cached_checksum_verified"`
}

func (a *app) planSync(symbols []string, issue string, force bool, interval time.Duration) ([]syncPlanRow, error) {
	if a.offline {
		return nil, errOffline
	}
	if interval < 0 {
		return nil, fmt.Errorf("interval must not be negative")
	}
	rows := []syncPlanRow{}
	issue = store.NormalizeIssue(issue)
	for i, symbol := range symbols {
		if i > 0 {
			if err := cdn.Pause(a.ctx, interval); err != nil {
				return nil, err
			}
		}
		symbol = strings.TrimSpace(symbol)
		pm, err := a.client().PubMedia(a.ctx, cdn.PubMediaQuery{Pub: symbol, Issue: issue, Format: "JWPUB"})
		if err != nil {
			return nil, err
		}
		file, ok := pm.JWPUB(a.lang)
		if !ok {
			return nil, fmt.Errorf("no JWPUB for %s %s", symbol, issue)
		}
		u, err := url.Parse(file.File.URL)
		if err != nil {
			return nil, err
		}
		row := syncPlanRow{Key: store.PubKey(symbol, a.lang, issue), Symbol: symbol, Issue: issue, Title: pm.PubName, File: filepath.Join(a.libDir, "pubs", path.Base(u.Path)), Checksum: file.File.Checksum, DownloadBytes: file.Filesize}
		if !force && row.Checksum != "" {
			if info, e := os.Stat(row.File); e == nil && info.Size() == file.Filesize {
				sum, e := cdn.FileMD5(row.File)
				row.Cached = e == nil && strings.EqualFold(sum, row.Checksum)
			}
		}
		if row.Cached {
			row.DownloadBytes = 0
		} else {
			row.EstimatedDiskBytes = file.Filesize * 3
		}
		rows = append(rows, row)
	}
	return rows, nil
}
func (a *app) printSyncPlan(rows []syncPlanRow) error {
	total, disk := int64(0), int64(0)
	for _, r := range rows {
		total += r.DownloadBytes
		disk += r.EstimatedDiskBytes
	}
	if a.jsonOut {
		return a.printJSON(map[string]any{"plan": rows, "download_bytes": total, "estimated_additional_disk_bytes": disk, "disk_estimate_multiplier": 3, "estimate_note": "Downloaded archive plus an estimated two archive sizes for index growth; actual growth varies."})
	}
	for _, r := range rows {
		a.printf("%s: download %d bytes, estimated additional disk %d bytes (cached: %t)\n", r.Key, r.DownloadBytes, r.EstimatedDiskBytes, r.Cached)
	}
	return nil
}
