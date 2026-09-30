package cli

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjuanrivvera/jwpubkit/internal/cdn"
)

func TestFrameTimes(t *testing.T) {
	for _, tc := range []struct {
		at              []string
		every, from, to string
		want            []time.Duration
	}{
		{at: []string{"1", "01:02.5", "00:00:03"}, want: []time.Duration{time.Second, 62500 * time.Millisecond, 3 * time.Second}},
		{every: "2", from: "1", to: "6", want: []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}},
		{every: "2", from: "0", to: "4", want: []time.Duration{0, 2 * time.Second, 4 * time.Second}},
		{every: "1", from: "4", to: "4", want: []time.Duration{4 * time.Second}},
	} {
		got, err := frameTimes(tc.at, tc.every, tc.from, tc.to)
		if err != nil || len(got) != len(tc.want) {
			t.Fatalf("%v %v", got, err)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%v != %v", got, tc.want)
			}
		}
	}
	for _, tc := range []struct {
		at              []string
		every, from, to string
	}{
		{}, {at: []string{"1"}, every: "2"}, {at: []string{"1"}, from: "0"}, {at: []string{"NaN"}}, {at: []string{"Inf"}}, {at: []string{"1e20"}}, {at: []string{"1:99"}}, {at: []string{"1:2:3:4"}}, {every: "0", from: "0", to: "1"}, {every: "1", from: "bad", to: "1"}, {every: "1", from: "0", to: "bad"}, {every: "1", from: "3", to: "1"}, {every: "0.001", from: "0", to: "1"}, {every: "1", from: "0"},
	} {
		if got, err := frameTimes(tc.at, tc.every, tc.from, tc.to); err == nil {
			t.Fatalf("accepted %+v: %v", tc, got)
		}
	}
}

func TestMediaFrameCommand(t *testing.T) {
	st := inventedLibrary(t)
	key := "pub-invented_1_VIDEO"
	item := cdn.MediaItem{LanguageAgnosticNaturalKey: key, Duration: 60, Files: []cdn.MediaFile{{Label: "240p", ProgressiveDownloadURL: "https://example.invalid/invented.mp4"}}}
	if err := st.PutVideo(key, "E", "Invented clockwork", 60, "", item); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		fail bool
	}{
		{[]string{"--at", "1"}, false},
		{[]string{"--at", "1", "--at", "2"}, false},
		{[]string{"--every", "2", "--from", "1", "--to", "5"}, false},
		{[]string{"--at", "60"}, true}, {[]string{"--at", "NaN"}, true}, {[]string{"--at", "1", "--offline"}, true}, {[]string{"--at", "1", "--resolution", "720p"}, true}, {[]string{"--at", "1", "--format", "webp"}, true}, {[]string{"--at", "1", "--traffic-bytes", "0"}, true}, {[]string{}, true},
	} {
		var out bytes.Buffer
		a := &app{ctx: t.Context(), out: &out, err: io.Discard, origins: map[string]string{}, st: st, clipRun: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			f, err := os.Create(args[len(args)-1])
			if err != nil {
				return nil, err
			}
			defer f.Close()
			return nil, png.Encode(f, image.NewRGBA(image.Rect(0, 0, 16, 9)))
		}}
		root := a.rootCmd()
		root.PersistentPostRunE = nil
		dest := filepath.Join(t.TempDir(), "batch")
		if len(tc.args) > 0 && tc.args[0] == "--at" && !strings.Contains(strings.Join(tc.args[2:], " "), "--at") {
			dest += ".png"
		}
		root.SetArgs(append([]string{"--library", st.Dir, "--language", "E", "--json", "media", "frame", key, "--format", "png", "--output", dest}, tc.args...))
		err := root.ExecuteContext(t.Context())
		if (err != nil) != tc.fail {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !tc.fail && !strings.Contains(out.String(), `"received_bytes": 0`) {
			t.Fatal(out.String())
		}
	}
}
