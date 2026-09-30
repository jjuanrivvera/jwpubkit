package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
	"github.com/jjuanrivvera/jwpubkit/internal/media"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

func clipTime(value string) (time.Duration, error) {
	parts := strings.Split(value, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("invalid time %q", value)
	}
	total := 0.0
	for i, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v < 0 || (i > 0 && v >= 60) {
			return 0, fmt.Errorf("invalid time %q", value)
		}
		total = total*60 + v
	}
	return time.Duration(total * float64(time.Second)), nil
}
func (a *app) mediaCmd() *cobra.Command {
	root := &cobra.Command{Use: "media", Short: "Work with audiovisual catalog items"}
	var from, to, resolution, mode, output string
	var traffic, space int64
	cmd := &cobra.Command{Use: "clip <video-key>", Short: "Cut a remote MP4 using bounded byte-range requests", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if a.offline {
			return errOffline
		}
		a.ctx = cmd.Context()
		key, err := subs.NormalizeKey(args[0])
		if err != nil {
			return err
		}
		first, err := clipTime(from)
		if err != nil {
			return err
		}
		last, err := clipTime(to)
		if err != nil {
			return err
		}
		st, err := a.commandStore(cmd)
		if err != nil {
			return err
		}
		var item cdn.MediaItem
		cached, err := st.Video(key, a.lang)
		if err != nil {
			return err
		}
		if cached != nil {
			if err := json.Unmarshal([]byte(cached.JSON), &item); err != nil {
				return err
			}
		} else {
			m, err := a.client().MediaItem(cmd.Context(), key)
			if err != nil {
				return err
			}
			item = *m
		}
		u := ""
		for _, f := range item.Files {
			if f.Label == resolution && f.ProgressiveDownloadURL != "" {
				u = f.ProgressiveDownloadURL
				break
			}
		}
		if u == "" {
			return fmt.Errorf("%s has no %s progressive rendition", key, resolution)
		}
		if item.Duration > 0 && last.Seconds() > item.Duration {
			return fmt.Errorf("requested end exceeds video duration")
		}
		result, err := media.Clip(cmd.Context(), media.Options{URL: u, From: first, To: last, Mode: mode, Output: output, TrafficBytes: traffic, OutputBytes: space, Client: a.client().HTTP, Run: a.clipRun})
		if a.jsonOut {
			if e := a.printJSON(map[string]any{"key": key, "source": u, "resolution": resolution, "clip": result}); e != nil {
				return e
			}
		} else if err == nil {
			a.printf("%s: %.3f s, %d output bytes, %d received bytes (%s)\n", output, result.Duration, result.OutputBytes, result.ReceivedBytes, mode)
		}
		return err
	}}
	cmd.Flags().StringVar(&from, "from", "", "start: seconds, mm:ss or hh:mm:ss")
	cmd.Flags().StringVar(&to, "to", "", "end: seconds, mm:ss or hh:mm:ss")
	cmd.Flags().StringVar(&resolution, "resolution", "240p", "progressive rendition label")
	cmd.Flags().StringVar(&mode, "mode", "copy", "copy (keyframe boundaries) or exact (reencode)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "new output MP4 path")
	cmd.Flags().Int64Var(&traffic, "traffic-bytes", 30000000, "maximum remote response-body bytes")
	cmd.Flags().Int64Var(&space, "output-bytes", 10000000, "maximum output bytes")
	root.AddCommand(cmd)
	return root
}
