// Package media makes bounded remote clips using a range-only loopback proxy.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Run executes a tool with the supplied cancellation context.
type Run func(context.Context, string, ...string) ([]byte, error)

// Options separates network traffic and output budgets, since they differ substantially.
type Options struct {
	URL                       string
	From, To                  time.Duration
	Mode, Output              string
	TrafficBytes, OutputBytes int64
	Client                    *http.Client
	Run                       Run
}

// Result records requested and effective times and measured response-body traffic.
type Result struct {
	Output        string  `json:"output"`
	Mode          string  `json:"mode"`
	FromSeconds   float64 `json:"from_seconds"`
	ToSeconds     float64 `json:"to_seconds"`
	Duration      float64 `json:"duration_seconds"`
	ReceivedBytes int64   `json:"received_bytes"`
	OutputBytes   int64   `json:"output_bytes"`
}

// Exec invokes ffmpeg or ffprobe without a shell.
func Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type rangeProxy struct {
	ctx      context.Context
	cancel   context.CancelFunc
	client   *http.Client
	source   string
	budget   int64
	mu       sync.Mutex
	received int64
	failure  error
	closing  bool
	handlers sync.WaitGroup
}

func (p *rangeProxy) fail(err error) {
	p.mu.Lock()
	if p.failure == nil {
		p.failure = err
	}
	p.mu.Unlock()
	p.cancel()
}
func (p *rangeProxy) snapshot() (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.received, p.failure
}
func (p *rangeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		return
	}
	p.handlers.Add(1)
	p.mu.Unlock()
	defer p.handlers.Done()
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	requested := r.Header.Get("Range")
	var start int64
	if !strings.HasPrefix(requested, "bytes=") || strings.Contains(requested, ",") {
		p.fail(errors.New("a single byte range is required"))
		return
	}
	bounds := strings.Split(strings.TrimPrefix(requested, "bytes="), "-")
	if len(bounds) != 2 {
		p.fail(errors.New("invalid byte range"))
		return
	}
	var err error
	start, err = strconv.ParseInt(bounds[0], 10, 64)
	if err != nil || start < 0 {
		p.fail(errors.New("invalid range start"))
		return
	}
	endRequested := int64(-1)
	if bounds[1] != "" {
		endRequested, err = strconv.ParseInt(bounds[1], 10, 64)
		if err != nil || endRequested < start {
			p.fail(errors.New("invalid range end"))
			return
		}
	}
	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, p.source, nil)
	if err != nil {
		p.fail(err)
		return
	}
	req.Header.Set("Range", requested)
	req.Header.Set("Accept-Encoding", "identity")
	response, err := p.client.Do(req)
	if err != nil {
		if p.ctx.Err() == nil {
			p.fail(err)
		}
		return
	}
	defer response.Body.Close()
	var first, last, total int64
	n, scanErr := fmt.Sscanf(response.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &total)
	if response.StatusCode != http.StatusPartialContent || scanErr != nil || n != 3 || first != start || last < first || last >= total || (endRequested >= 0 && last > endRequested) || response.ContentLength != last-first+1 {
		p.fail(fmt.Errorf("server did not respect Range %q: HTTP %d, Content-Range %q", requested, response.StatusCode, response.Header.Get("Content-Range")))
		return
	}
	w.Header().Set("Content-Range", response.Header.Get("Content-Range"))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(last-first+1, 10))
	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusPartialContent)
	buffer := make([]byte, 32*1024)
	streamRead := int64(0)
	for {
		// Serializing reads makes the byte cap apply across overlapping ffmpeg requests.
		p.mu.Lock()
		remaining := p.budget - p.received
		if remaining <= 0 {
			p.mu.Unlock()
			p.fail(errors.New("traffic byte budget exhausted"))
			return
		}
		size := int64(len(buffer))
		if remaining < size {
			size = remaining
		}
		n, readErr := response.Body.Read(buffer[:size])
		p.received += int64(n)
		streamRead += int64(n)
		p.mu.Unlock()
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
		}
		if streamRead == last-first+1 {
			return
		}
		if readErr == io.EOF {
			return
		}
		if readErr != nil {
			if p.ctx.Err() == nil {
				p.fail(readErr)
			}
			return
		}
	}
}

// Clip rejects a server that sends full files for ranges, and publishes only a successful bounded output.
func Clip(ctx context.Context, opt Options) (Result, error) {
	result := Result{Output: opt.Output, Mode: opt.Mode, FromSeconds: opt.From.Seconds(), ToSeconds: opt.To.Seconds()}
	if opt.From < 0 || opt.To <= opt.From {
		return result, errors.New("to must be after a nonnegative from")
	}
	if opt.Mode != "copy" && opt.Mode != "exact" {
		return result, errors.New("mode must be copy or exact")
	}
	if opt.TrafficBytes <= 0 || opt.OutputBytes < 131072 {
		return result, errors.New("traffic budget must be positive and output budget at least 131072 bytes")
	}
	if opt.Client == nil {
		return result, errors.New("HTTP client is required")
	}
	if opt.Output == "" {
		return result, errors.New("output path is required")
	}
	if _, err := os.Stat(opt.Output); err == nil {
		return result, errors.New("output already exists")
	}
	if opt.Run == nil {
		opt.Run = Exec
	}
	if err := os.MkdirAll(filepath.Dir(opt.Output), 0o755); err != nil {
		return result, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(opt.Output), ".pubkit-clip-*.mp4")
	if err != nil {
		return result, err
	}
	temp := tmp.Name()
	if err := tmp.Close(); err != nil {
		return result, err
	}
	defer os.Remove(temp)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	proxy := &rangeProxy{ctx: runCtx, cancel: cancel, client: opt.Client, source: opt.URL, budget: opt.TrafficBytes}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return result, err
	}
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second,
		ConnState: func(conn net.Conn, state http.ConnState) {
			// Keep loopback buffering small so a seek does not prefetch megabytes the player discards.
			if state == http.StateNew {
				if tcp, ok := conn.(*net.TCPConn); ok {
					_ = tcp.SetWriteBuffer(32 * 1024)
				}
			}
		}}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	var stopped sync.Once
	stopProxy := func() {
		stopped.Do(func() {
			proxy.mu.Lock()
			proxy.closing = true
			proxy.mu.Unlock()
			cancel()
			_ = server.Close()
			proxy.handlers.Wait()
			<-done
		})
	}
	defer stopProxy()
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-ss", strconv.FormatFloat(opt.From.Seconds(), 'f', 3, 64), "-i", "http://" + listener.Addr().String() + "/source.mp4", "-t", strconv.FormatFloat((opt.To - opt.From).Seconds(), 'f', 3, 64)}
	if opt.Mode == "copy" {
		args = append(args, "-c", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-c:a", "aac")
	}
	args = append(args, "-fs", strconv.FormatInt(opt.OutputBytes-65536, 10), "-movflags", "+faststart", temp)
	b, runErr := opt.Run(runCtx, "ffmpeg", args...)
	stopProxy()
	result.ReceivedBytes, err = proxy.snapshot()
	if err != nil {
		return result, err
	}
	if runErr != nil {
		return result, fmt.Errorf("ffmpeg: %w: %s", runErr, strings.TrimSpace(string(b)))
	}
	stat, err := os.Stat(temp)
	if err != nil {
		return result, err
	}
	result.OutputBytes = stat.Size()
	if result.OutputBytes == 0 || result.OutputBytes > opt.OutputBytes {
		return result, errors.New("output byte budget exceeded or empty clip")
	}
	probe, err := opt.Run(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", temp)
	if err != nil {
		return result, fmt.Errorf("ffprobe: %w", err)
	}
	var data struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(probe, &data); err != nil {
		return result, err
	}
	result.Duration, err = strconv.ParseFloat(data.Format.Duration, 64)
	if err != nil {
		return result, err
	}
	expected := (opt.To - opt.From).Seconds()
	tolerance := 1.0
	if opt.Mode == "exact" {
		tolerance = 0.15
	}
	if result.Duration <= 0 || result.Duration < expected-tolerance {
		return result, errors.New("clip truncated before the requested end (output budget may be too small)")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, os.Rename(temp, opt.Output)
}
