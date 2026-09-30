package media

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FrameOptions uses one network budget and one disk budget for the entire batch.
type FrameOptions struct {
	URL, Output, Format, ContactSheet string
	Times                             []time.Duration
	Columns                           int
	TrafficBytes, OutputBytes         int64
	Interval                          time.Duration
	Client                            *http.Client
	Run                               Run
}

type Frame struct {
	Output      string  `json:"output"`
	AtSeconds   float64 `json:"at_seconds"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	OutputBytes int64   `json:"output_bytes"`
}

type FrameResult struct {
	Frames        []Frame `json:"frames"`
	ContactSheet  string  `json:"contact_sheet,omitempty"`
	ReceivedBytes int64   `json:"received_bytes"`
	OutputBytes   int64   `json:"output_bytes"`
}

// Frames stages and validates the entire batch before publishing any image.
func Frames(ctx context.Context, opt FrameOptions) (result FrameResult, err error) {
	if len(opt.Times) == 0 || len(opt.Times) > 100 {
		return result, errors.New("request between 1 and 100 frames")
	}
	for _, at := range opt.Times {
		if at < 0 {
			return result, errors.New("frame time must be nonnegative")
		}
	}
	if opt.Format != "jpg" && opt.Format != "png" {
		return result, errors.New("format must be jpg or png")
	}
	if opt.Output == "" || opt.Client == nil || opt.TrafficBytes <= 0 || opt.OutputBytes <= 0 || opt.Interval < 0 || opt.Columns < 1 || opt.Columns > 10 {
		return result, errors.New("invalid frame output, client, budgets, interval or columns")
	}
	if opt.ContactSheet != "" && !strings.EqualFold(filepath.Ext(opt.ContactSheet), ".jpg") && !strings.EqualFold(filepath.Ext(opt.ContactSheet), ".png") {
		return result, errors.New("contact sheet must have a .jpg or .png extension")
	}
	if len(opt.Times) == 1 && !strings.EqualFold(filepath.Ext(opt.Output), "."+opt.Format) {
		return result, errors.New("output extension must match frame format")
	}
	dir := filepath.Dir(opt.Output)
	if len(opt.Times) > 1 {
		dir = opt.Output
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(dir, ".pubkit-frames-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stage)
	destinations := make([]string, len(opt.Times))
	sources := make([]string, len(opt.Times))
	for i, at := range opt.Times {
		name := fmt.Sprintf("frame_%03d_%012d.%s", i+1, at.Milliseconds(), opt.Format)
		destinations[i] = filepath.Join(dir, name)
		if len(opt.Times) == 1 {
			destinations[i] = opt.Output
		}
		sources[i] = filepath.Join(stage, name)
	}
	if opt.ContactSheet != "" {
		destinations = append(destinations, opt.ContactSheet)
	}
	seen := map[string]bool{}
	for _, dest := range destinations {
		abs, err := filepath.Abs(dest)
		if err != nil {
			return result, err
		}
		if seen[abs] {
			return result, errors.New("output paths must be distinct")
		}
		seen[abs] = true
		if _, err := os.Lstat(dest); !os.IsNotExist(err) {
			return result, fmt.Errorf("output unavailable or already exists: %s", dest)
		}
	}
	if opt.Run == nil {
		opt.Run = Exec
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	proxy := &rangeProxy{ctx: runCtx, cancel: cancel, client: opt.Client, source: opt.URL, budget: opt.TrafficBytes, cache: &blockCache{blocks: map[int64][]byte{}, interval: opt.Interval}}
	input, stop, err := startProxy(proxy)
	if err != nil {
		return result, err
	}
	defer stop()
	for i, at := range opt.Times {
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-ss", strconv.FormatFloat(at.Seconds(), 'f', 6, 64), "-analyzeduration", "0", "-probesize", "32768", "-i", input, "-map", "0:v:0", "-frames:v", "1", "-an", "-fs", strconv.FormatInt(opt.OutputBytes-result.OutputBytes, 10)}
		if opt.Format == "jpg" {
			args = append(args, "-q:v", "2")
		}
		args = append(args, sources[i])
		b, runErr := opt.Run(runCtx, "ffmpeg", args...)
		result.ReceivedBytes, err = proxy.snapshot()
		if err != nil {
			return result, err
		}
		if runErr != nil {
			return result, fmt.Errorf("ffmpeg: %w: %s", runErr, strings.TrimSpace(string(b)))
		}
		frame, err := inspectFrame(sources[i], opt.Format, opt.OutputBytes-result.OutputBytes)
		if err != nil {
			return result, err
		}
		frame.Output, frame.AtSeconds = destinations[i], at.Seconds()
		result.OutputBytes += frame.OutputBytes
		result.Frames = append(result.Frames, frame)
	}
	stop()
	result.ReceivedBytes, err = proxy.snapshot()
	if err != nil {
		return result, err
	}
	if opt.ContactSheet != "" {
		sheet := filepath.Join(stage, "contact"+strings.ToLower(filepath.Ext(opt.ContactSheet)))
		if err := contactSheet(sources, opt.Times, opt.Columns, sheet); err != nil {
			return result, err
		}
		stat, err := os.Stat(sheet)
		if err != nil {
			return result, err
		}
		result.OutputBytes += stat.Size()
		sources = append(sources, sheet)
		result.ContactSheet = opt.ContactSheet
	}
	if result.OutputBytes > opt.OutputBytes {
		return result, errors.New("frame output byte budget exceeded")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// Exclusive creation also prevents a concurrent writer from being overwritten.
	published := []string{}
	defer func() {
		if err != nil {
			for _, dest := range published {
				_ = os.Remove(dest)
			}
		}
	}()
	for i, source := range sources {
		if err = publishImage(source, destinations[i]); err != nil {
			return result, err
		}
		published = append(published, destinations[i])
	}
	return result, nil
}

func inspectFrame(path, format string, budget int64) (Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return Frame{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Frame{}, err
	}
	if stat.Size() <= 0 || stat.Size() > budget {
		return Frame{}, errors.New("empty frame or frame output byte budget exceeded")
	}
	cfg, kind, err := image.DecodeConfig(f)
	if err != nil {
		return Frame{}, fmt.Errorf("invalid frame: %w", err)
	}
	if format == "jpg" {
		format = "jpeg"
	}
	if kind != format || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 80000000 {
		return Frame{}, errors.New("invalid frame format or dimensions")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return Frame{}, err
	}
	if _, _, err := image.Decode(f); err != nil {
		return Frame{}, fmt.Errorf("incomplete frame: %w", err)
	}
	return Frame{Width: cfg.Width, Height: cfg.Height, OutputBytes: stat.Size()}, nil
}

func publishImage(source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(dest)
		return errors.Join(writeErr, closeErr)
	}
	return nil
}

func contactSheet(paths []string, times []time.Duration, columns int, output string) error {
	if columns > len(paths) {
		columns = len(paths)
	}
	const width = 320
	first, err := os.Open(paths[0])
	if err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(first)
	_ = first.Close()
	if err != nil {
		return err
	}
	height := max(1, min(640, width*cfg.Height/cfg.Width))
	cellHeight := height + 24
	rows := (len(paths) + columns - 1) / columns
	sheet := image.NewRGBA(image.Rect(0, 0, width*columns, cellHeight*rows))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	for i, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		img, _, decodeErr := image.Decode(f)
		_ = f.Close()
		if decodeErr != nil {
			return decodeErr
		}
		bounds := img.Bounds()
		// Fit images without distorting their aspect ratio; thumbnails need no extra dependency.
		scale := math.Min(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy()))
		w, h := max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))
		left, top := i%columns*width+(width-w)/2, i/columns*cellHeight+(height-h)/2
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sheet.Set(left+x, top+y, img.At(bounds.Min.X+x*bounds.Dx()/w, bounds.Min.Y+y*bounds.Dy()/h))
			}
		}
		millis := times[i].Milliseconds()
		label := fmt.Sprintf("%02d:%02d:%02d.%03d", millis/3600000, millis/60000%60, millis/1000%60, millis%1000)
		drawTime(sheet, i%columns*width+8, i/columns*cellHeight+height+5, label)
	}
	f, err := os.Create(output)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(output), ".png") {
		err = png.Encode(f, sheet)
	} else {
		err = jpeg.Encode(f, sheet, &jpeg.Options{Quality: 90})
	}
	return errors.Join(err, f.Close())
}

// A tiny timestamp alphabet keeps contact sheets portable without font assets.
func drawTime(dst *image.RGBA, x, y int, text string) {
	glyphs := map[rune]string{'0': "111101101101111", '1': "010110010010111", '2': "111001111100111", '3': "111001111001111", '4': "101101111001001", '5': "111100111001111", '6': "111100111101111", '7': "111001001001001", '8': "111101111101111", '9': "111101111001111", ':': "000010000010000", '.': "000000000000010"}
	for _, ch := range text {
		for i, bit := range glyphs[ch] {
			if bit == '1' {
				for dy := 0; dy < 2; dy++ {
					for dx := 0; dx < 2; dx++ {
						dst.Set(x+(i%3)*2+dx, y+(i/3)*2+dy, color.White)
					}
				}
			}
		}
		x += 8
	}
}
