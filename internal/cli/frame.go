package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jjuanrivvera/jwpubkit/internal/media"
	"github.com/jjuanrivvera/jwpubkit/internal/subs"
)

func frameTimes(at []string, every, from, to string) ([]time.Duration, error) {
	if len(at) > 0 && (every != "" || from != "" || to != "") {
		return nil, fmt.Errorf("use --at or --every with --from and --to")
	}
	times := []time.Duration{}
	if len(at) > 0 {
		for _, mark := range at {
			t, err := clipTime(mark)
			if err != nil {
				return nil, err
			}
			times = append(times, t)
		}
	} else {
		if every == "" || from == "" || to == "" {
			return nil, fmt.Errorf("provide --at or --every with --from and --to")
		}
		step, err := clipTime(every)
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("every must be a positive number of seconds or clock time")
		}
		first, err := clipTime(from)
		if err != nil {
			return nil, err
		}
		last, err := clipTime(to)
		if err != nil {
			return nil, err
		}
		if last < first {
			return nil, fmt.Errorf("to must be at or after from")
		}
		count := (last-first)/step + 1
		if count > 100 {
			return nil, fmt.Errorf("request at most 100 frames")
		}
		for i := time.Duration(0); i < count; i++ {
			times = append(times, first+i*step)
		}
	}
	if len(times) > 100 {
		return nil, fmt.Errorf("request at most 100 frames")
	}
	return times, nil
}

func (a *app) frameCmd() *cobra.Command {
	var at []string
	var every, from, to, resolution, output, format, sheet string
	var traffic, space int64
	var columns int
	var interval time.Duration
	cmd := &cobra.Command{Use: "frame <video-key>", Short: "Extract JPG/PNG frames through cached, bounded byte ranges", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if a.offline {
			return errOffline
		}
		times, err := frameTimes(at, every, from, to)
		if err != nil {
			return err
		}
		if format != "jpg" && format != "png" {
			return fmt.Errorf("format must be jpg or png")
		}
		if traffic <= 0 || space <= 0 || interval < 0 || columns < 1 || columns > 10 {
			return fmt.Errorf("invalid frame budgets, interval or columns")
		}
		key, err := subs.NormalizeKey(args[0])
		if err != nil {
			return err
		}
		a.ctx = cmd.Context()
		item, source, err := a.mediaSource(cmd, key, resolution)
		if err != nil {
			return err
		}
		for _, at := range times {
			if item.Duration > 0 && at.Seconds() >= item.Duration {
				return fmt.Errorf("frame time must be before video end")
			}
		}
		dest := output
		if dest == "" {
			dest = key + "-frames"
			if len(times) == 1 {
				dest = fmt.Sprintf("%s_%012d.%s", key, times[0].Milliseconds(), format)
			}
		}
		dest = filepath.Clean(dest)
		result, err := media.Frames(cmd.Context(), media.FrameOptions{URL: source, Output: dest, Format: format, ContactSheet: sheet, Times: times, Columns: columns, TrafficBytes: traffic, OutputBytes: space, Interval: interval, Client: a.client().HTTP, Run: a.clipRun})
		if a.jsonOut {
			if e := a.printJSON(map[string]any{"key": key, "source": source, "resolution": resolution, "frames": result}); e != nil {
				return e
			}
		} else if err == nil {
			for _, f := range result.Frames {
				a.printf("%s: %.3f s, %dx%d, %d bytes\n", f.Output, f.AtSeconds, f.Width, f.Height, f.OutputBytes)
			}
			if result.ContactSheet != "" {
				a.printf("Contact sheet: %s\n", result.ContactSheet)
			}
			a.printf("%d output bytes, %d received bytes\n", result.OutputBytes, result.ReceivedBytes)
		}
		return err
	}}
	cmd.Flags().StringArrayVar(&at, "at", nil, "frame time: seconds, mm:ss or hh:mm:ss (repeatable)")
	cmd.Flags().StringVar(&every, "every", "", "interval in seconds or clock time; requires --from and --to")
	cmd.Flags().StringVar(&from, "from", "", "first frame time for --every")
	cmd.Flags().StringVar(&to, "to", "", "last allowed frame time for --every (inclusive)")
	cmd.Flags().StringVar(&resolution, "resolution", "240p", "progressive rendition label")
	cmd.Flags().StringVar(&format, "format", "jpg", "jpg or png")
	cmd.Flags().StringVarP(&output, "output", "o", "", "new image path for one mark, or directory for multiple marks")
	cmd.Flags().StringVar(&sheet, "contact-sheet", "", "optional new .jpg or .png contact sheet with timestamps")
	cmd.Flags().IntVar(&columns, "columns", 3, "contact sheet columns (1-10)")
	cmd.Flags().Int64Var(&traffic, "traffic-bytes", 30000000, "maximum remote response-body bytes across all marks")
	cmd.Flags().Int64Var(&space, "output-bytes", 20000000, "maximum total image and contact sheet bytes")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "minimum pause between uncached remote range requests")
	return cmd
}
