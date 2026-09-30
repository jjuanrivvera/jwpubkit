package cdn

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCategoriesAndLimited(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/categories/E":
			_, _ = io.WriteString(w, `{"category":{"subcategories":[{"key":"invented"}]}}`)
		case "/categories/E/invented":
			_, _ = io.WriteString(w, `{"category":{"key":"invented","media":[{"languageAgnosticNaturalKey":"pub-invented_1_VIDEO"}]}}`)
		default:
			_, _ = io.WriteString(w, "invented body")
		}
	}))
	defer source.Close()
	client := New("E")
	client.HTTP = source.Client()
	client.CategoriesURL = source.URL + "/categories"
	root, err := client.Categories(t.Context(), "")
	if err != nil || len(root.Subcategories) != 1 {
		t.Fatalf("%+v %v", root, err)
	}
	category, err := client.Categories(t.Context(), "invented")
	if err != nil || len(category.Media) != 1 {
		t.Fatalf("%+v %v", category, err)
	}
	sum := md5.Sum([]byte("invented body"))
	want := hex.EncodeToString(sum[:])
	b, n, err := client.GetLimited(t.Context(), source.URL, 100, want)
	if err != nil || string(b) != "invented body" || n != 13 {
		t.Fatalf("%s %d %v", b, n, err)
	}
	for _, test := range []struct {
		budget   int64
		checksum string
	}{{0, ""}, {2, ""}, {100, "bad"}} {
		if _, _, err := client.GetLimited(t.Context(), source.URL, test.budget, test.checksum); err == nil {
			t.Fatal("accepted invalid download")
		}
	}
	if err := Pause(t.Context(), -time.Second); err == nil {
		t.Fatal("negative pause")
	}
	if err := Pause(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
}
