package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func inventedImage(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, 16, 9))
	img.Set(2, 3, color.RGBA{R: 200, A: 255})
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if strings.HasSuffix(path, ".png") {
		err = png.Encode(f, img)
	} else {
		err = jpeg.Encode(f, img, nil)
	}
	return errors.Join(err, f.Close())
}

func TestFramesReuseRangesAndSheet(t *testing.T) {
	for _, format := range []string{"jpg", "png"} {
		t.Run(format, func(t *testing.T) {
			var requests atomic.Int64
			data := bytes.Repeat([]byte("invented clockwork bytes"), 20000)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("Range") == "" || strings.HasSuffix(r.Header.Get("Range"), "-") {
					t.Errorf("unbounded range %q", r.Header.Get("Range"))
				}
				http.ServeContent(w, r, "invented.mp4", time.Time{}, bytes.NewReader(data))
			}))
			defer server.Close()
			runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if name != "ffmpeg" {
					return nil, fmt.Errorf("unexpected tool %s", name)
				}
				input := ""
				for i, a := range args {
					if a == "-i" {
						input = args[i+1]
					}
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Range", "bytes=1-15")
				resp, err := server.Client().Do(req)
				if err != nil {
					return nil, err
				}
				_, err = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return nil, err
				}
				return nil, inventedImage(args[len(args)-1])
			}
			dir := t.TempDir()
			sheet := filepath.Join(dir, "sheet."+format)
			got, err := Frames(t.Context(), FrameOptions{URL: server.URL, Output: filepath.Join(dir, "frames"), Format: format, ContactSheet: sheet, Times: []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}, Columns: 2, TrafficBytes: 1000000, OutputBytes: 1000000, Client: server.Client(), Run: runner})
			if err != nil || len(got.Frames) != 3 || requests.Load() != 1 || got.ReceivedBytes != rangeBlock {
				t.Fatalf("%+v requests=%d %v", got, requests.Load(), err)
			}
			if got.Frames[0].Width != 16 || got.Frames[0].Height != 9 || got.Frames[2].AtSeconds != 3 {
				t.Fatalf("%+v", got)
			}
			f, err := os.Open(sheet)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, _, err := image.Decode(f)
			if err != nil || img.Bounds().Dx() != 640 || img.Bounds().Dy() != 408 {
				t.Fatalf("sheet %v %v", img.Bounds(), err)
			}
			// The label strip must contain visible timestamp pixels.
			nonblack := false
			for y := 185; y < 195; y++ {
				for x := 8; x < 100; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r+g+b > 10000 {
						nonblack = true
					}
				}
			}
			if !nonblack {
				t.Fatal("contact sheet has no timestamp")
			}
		})
	}
}

func TestFrameValidationAndAtomicFailure(t *testing.T) {
	base := FrameOptions{URL: "http://example.invalid", Output: filepath.Join(t.TempDir(), "out.jpg"), Format: "jpg", Times: []time.Duration{time.Second}, Columns: 3, TrafficBytes: 10000, OutputBytes: 10000, Client: &http.Client{}, Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		return nil, inventedImage(args[len(args)-1])
	}}
	for _, change := range []func(*FrameOptions){
		func(o *FrameOptions) { o.Times = nil }, func(o *FrameOptions) { o.Times = make([]time.Duration, 101) }, func(o *FrameOptions) { o.Times = []time.Duration{-1} }, func(o *FrameOptions) { o.Format = "webp" }, func(o *FrameOptions) { o.Output = "" }, func(o *FrameOptions) { o.Client = nil }, func(o *FrameOptions) { o.TrafficBytes = 0 }, func(o *FrameOptions) { o.OutputBytes = 0 }, func(o *FrameOptions) { o.Interval = -1 }, func(o *FrameOptions) { o.Columns = 11 }, func(o *FrameOptions) { o.ContactSheet = "bad.webp" }, func(o *FrameOptions) { o.Output = "bad.png" }, func(o *FrameOptions) { o.ContactSheet = o.Output },
	} {
		opt := base
		change(&opt)
		if _, err := Frames(t.Context(), opt); err == nil {
			t.Fatalf("accepted %+v", opt)
		}
	}
	if _, err := Frames(t.Context(), base); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(base.Output)
	if _, err := Frames(t.Context(), base); err == nil {
		t.Fatal("overwrote image")
	}
	after, _ := os.ReadFile(base.Output)
	if !bytes.Equal(before, after) {
		t.Fatal("changed existing output")
	}
	for _, scenario := range []string{"tool", "empty", "corrupt", "budget", "second", "sheet-budget", "cancel", "race"} {
		t.Run(scenario, func(t *testing.T) {
			o := base
			o.Output = filepath.Join(t.TempDir(), "batch")
			o.Times = []time.Duration{time.Second, 2 * time.Second}
			if scenario == "budget" {
				o.OutputBytes = 1
			}
			if scenario == "sheet-budget" {
				o.OutputBytes = 1500
				o.ContactSheet = filepath.Join(t.TempDir(), "sheet.png")
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			o.Run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				calls++
				path := args[len(args)-1]
				switch scenario {
				case "tool":
					return nil, errors.New("invented failure")
				case "empty":
					return nil, os.WriteFile(path, nil, 0o600)
				case "corrupt":
					return nil, os.WriteFile(path, []byte("invented invalid image"), 0o600)
				case "second":
					if calls == 2 {
						return nil, errors.New("invented failure")
					}
				case "cancel":
					cancel()
				case "race":
					if calls == 2 {
						dest := filepath.Join(o.Output, "frame_002_000000002000.jpg")
						if err := os.WriteFile(dest, []byte("existing concurrent output"), 0o600); err != nil {
							return nil, err
						}
					}
				}
				return nil, inventedImage(path)
			}
			if _, err := Frames(ctx, o); err == nil {
				t.Fatal("accepted failed batch")
			}
			files, err := os.ReadDir(o.Output)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if scenario == "race" {
				want = 1
			}
			if len(files) != want {
				t.Fatalf("published failed batch: %v", files)
			}
		})
	}
}

func TestCachedRangeSafety(t *testing.T) {
	for _, scenario := range []string{"ignored", "wrong", "short", "budget", "tail", "two-blocks", "method", "cancel", "interval"} {
		t.Run(scenario, func(t *testing.T) {
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "ignored" {
					w.WriteHeader(http.StatusOK)
					return
				}
				var first, last int64
				_, _ = fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &first, &last)
				total := rangeBlock + 32
				if last >= total {
					last = total - 1
				}
				if scenario == "wrong" {
					first++
				}
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, total))
				w.Header().Set("Content-Length", strconv.FormatInt(last-first+1, 10))
				w.WriteHeader(http.StatusPartialContent)
				n := last - first + 1
				if scenario == "short" {
					n = 1
				}
				_, _ = w.Write(bytes.Repeat([]byte("x"), int(n)))
			}))
			defer source.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			budget := int64(1000000)
			if scenario == "budget" {
				budget = 8
			}
			p := &rangeProxy{ctx: ctx, cancel: cancel, client: source.Client(), source: source.URL, budget: budget, cache: &blockCache{blocks: map[int64][]byte{}}}
			r := httptest.NewRequest(http.MethodGet, "http://local/source", nil)
			r.Header.Set("Range", "bytes=0-")
			if scenario == "tail" {
				r.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeBlock+1))
			}
			if scenario == "method" {
				r.Method = http.MethodHead
			}
			if scenario == "cancel" {
				cancel()
				r = r.WithContext(ctx)
			}
			if scenario == "interval" {
				p.cache.last = time.Now()
				p.cache.interval = time.Hour
				cancel()
				r = r.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			p.ServeHTTP(w, r)
			n, err := p.snapshot()
			if n > budget {
				t.Fatalf("budget exceeded %d", n)
			}
			bad := scenario == "ignored" || scenario == "wrong" || scenario == "short" || scenario == "budget"
			if bad && err == nil {
				t.Fatal("accepted invalid range")
			}
			if scenario == "two-blocks" && (n != rangeBlock+32 || len(w.Body.Bytes()) != int(n)) {
				t.Fatalf("%d %d %v", n, w.Body.Len(), err)
			}
			if scenario == "tail" && (n != 32 || w.Body.Len() != 31) {
				t.Fatalf("tail %d %d %v", n, w.Body.Len(), err)
			}
		})
	}
}

func TestClipAudioModes(t *testing.T) {
	for _, mode := range []string{"copy", "exact"} {
		for _, audioOnly := range []bool{false, true} {
			o := Options{URL: "http://example.invalid", From: time.Second, To: 21 * time.Second, Mode: mode, Output: filepath.Join(t.TempDir(), "out.mp4"), TrafficBytes: 32, OutputBytes: 131072, Client: &http.Client{}, NoAudio: !audioOnly, AudioOnly: audioOnly}
			if audioOnly {
				o.Output = filepath.Join(t.TempDir(), "out.m4a")
			}
			o.Run = func(_ context.Context, tool string, args ...string) ([]byte, error) {
				if tool == "ffprobe" {
					return []byte(`{"format":{"duration":"20"}}`), nil
				}
				flags := strings.Join(args, " ")
				if audioOnly {
					if !strings.Contains(flags, "-map 0:a:0 -vn") || strings.Contains(flags, "libx264") || !strings.HasSuffix(args[len(args)-1], ".m4a") {
						t.Fatalf("audio flags %s", flags)
					}
				} else if !strings.Contains(flags, "-map 0:v:0 -an") || strings.Contains(flags, "-c:a") {
					t.Fatalf("mute flags %s", flags)
				}
				return nil, os.WriteFile(args[len(args)-1], []byte("invented clip"), 0o600)
			}
			got, err := Clip(t.Context(), o)
			if err != nil || got.AudioOnly != audioOnly || got.NoAudio == audioOnly {
				t.Fatalf("%+v %v", got, err)
			}
			o.NoAudio = true
			o.AudioOnly = true
			if _, err := Clip(t.Context(), o); err == nil {
				t.Fatal("accepted contradictory flags")
			}
			o.NoAudio = false
			o.Output = "bad.mp4"
			if _, err := Clip(t.Context(), o); err == nil {
				t.Fatal("accepted non-M4A audio output")
			}
		}
	}
}

func TestCachedRangePacing(t *testing.T) {
	starts := make(chan time.Time, 3)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		starts <- time.Now()
		http.ServeContent(w, r, "invented", time.Time{}, bytes.NewReader(bytes.Repeat([]byte("x"), int(rangeBlock+32))))
	}))
	defer source.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p := &rangeProxy{ctx: ctx, cancel: cancel, client: source.Client(), source: source.URL, budget: 1000000, cache: &blockCache{blocks: map[int64][]byte{}, interval: 50 * time.Millisecond}}
	if _, _, err := p.block(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.block(ctx, rangeBlock); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.block(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if len(starts) != 2 {
		t.Fatalf("cache miss: %d requests", len(starts))
	}
	first, second := <-starts, <-starts
	if second.Sub(first) < 40*time.Millisecond {
		t.Fatalf("unpacing: %v", second.Sub(first))
	}
}
