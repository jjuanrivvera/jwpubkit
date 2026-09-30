package media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClip(t *testing.T) {
	for _, mode := range []string{"copy", "exact"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "bytes=0-" {
					t.Errorf("range %q", r.Header.Get("Range"))
				}
				w.Header().Set("Content-Range", "bytes 0-31/32")
				w.Header().Set("Content-Length", "32")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, strings.Repeat("x", 32))
			}))
			defer server.Close()
			runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if name == "ffprobe" {
					return []byte(`{"format":{"duration":"20.000"}}`), nil
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
				req.Header.Set("Range", "bytes=0-")
				resp, err := server.Client().Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, err
				}
				return nil, os.WriteFile(args[len(args)-1], b, 0o600)
			}
			dest := filepath.Join(t.TempDir(), "out.mp4")
			got, err := Clip(t.Context(), Options{URL: server.URL, From: time.Second, To: 21 * time.Second, Mode: mode, Output: dest, TrafficBytes: 64, OutputBytes: 131072, Client: server.Client(), Run: runner})
			if err != nil || got.ReceivedBytes != 32 || got.OutputBytes != 32 || got.Duration != 20 {
				t.Fatalf("%+v %v", got, err)
			}
			if _, err := os.Stat(dest); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRangeProxyRejects(t *testing.T) {
	for _, name := range []string{"no-range", "malformed", "suffix", "multiple", "negative", "bad-end", "ignored", "wrong-range", "wrong-length", "budget"} {
		t.Run(name, func(t *testing.T) {
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "ignored" {
					w.WriteHeader(http.StatusOK)
					return
				}
				header := "bytes 0-31/32"
				if name == "wrong-range" {
					header = "bytes 1-31/32"
				}
				w.Header().Set("Content-Range", header)
				length := "32"
				if name == "wrong-length" {
					length = "31"
				}
				w.Header().Set("Content-Length", length)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, strings.Repeat("x", 32))
			}))
			defer source.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			budget := int64(64)
			if name == "budget" {
				budget = 8
			}
			proxy := &rangeProxy{ctx: ctx, cancel: cancel, client: source.Client(), source: source.URL, budget: budget}
			req := httptest.NewRequest(http.MethodGet, "http://local/source", nil)
			rangeValue := "bytes=0-"
			switch name {
			case "no-range":
				rangeValue = ""
			case "malformed":
				rangeValue = "bytes=0"
			case "suffix":
				rangeValue = "bytes=-10"
			case "multiple":
				rangeValue = "bytes=0-1,3-4"
			case "negative":
				rangeValue = "bytes=-1-"
			case "bad-end":
				rangeValue = "bytes=3-2"
			}
			req.Header.Set("Range", rangeValue)
			proxy.ServeHTTP(httptest.NewRecorder(), req)
			n, err := proxy.snapshot()
			if err == nil {
				t.Fatal("accepted unsafe range")
			}
			if n > budget {
				t.Fatalf("received %d > %d", n, budget)
			}
		})
	}
}
func TestClipValidationAndFailures(t *testing.T) {
	base := Options{URL: "http://example.invalid", From: 0, To: time.Second, Mode: "copy", Output: filepath.Join(t.TempDir(), "out.mp4"), TrafficBytes: 10, OutputBytes: 131072, Client: &http.Client{}}
	for _, change := range []func(*Options){func(o *Options) { o.From = -1 }, func(o *Options) { o.To = 0 }, func(o *Options) { o.Mode = "bad" }, func(o *Options) { o.TrafficBytes = 0 }, func(o *Options) { o.OutputBytes = 1 }, func(o *Options) { o.Client = nil }, func(o *Options) { o.Output = "" }} {
		o := base
		change(&o)
		if _, err := Clip(t.Context(), o); err == nil {
			t.Fatal("accepted invalid options")
		}
	}
	for _, name := range []string{"tool", "probe", "empty", "oversize", "duration", "json"} {
		t.Run(name, func(t *testing.T) {
			o := base
			o.Output = filepath.Join(t.TempDir(), "out.mp4")
			o.Run = func(_ context.Context, tool string, args ...string) ([]byte, error) {
				if name == "tool" || name == "probe" && tool == "ffprobe" {
					return nil, errors.New("invented failure")
				}
				if tool == "ffmpeg" {
					size := 8
					if name == "empty" {
						size = 0
					}
					if name == "oversize" {
						size = 131073
					}
					return nil, os.WriteFile(args[len(args)-1], []byte(strings.Repeat("x", size)), 0o600)
				}
				if name == "json" {
					return []byte("invalid"), nil
				}
				return []byte(`{"format":{"duration":"0"}}`), nil
			}
			if _, err := Clip(t.Context(), o); err == nil {
				t.Fatal("accepted failed/truncated clip")
			}
			if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
				t.Fatal("failed clip was published")
			}
		})
	}
	if _, err := Exec(t.Context(), "pubkit-invented-missing-tool"); err == nil {
		t.Fatal("missing tool succeeded")
	}
}
