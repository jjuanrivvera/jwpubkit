package store

import (
	"path/filepath"
	"testing"
)

// La precedencia importa porque hay bibliotecas ya sincronizadas: JWPUBKIT_HOME es el
// nombre actual, pero JWLIB_HOME tiene que seguir funcionando y XDG_DATA_HOME debe
// ganarle al ~/.local/share literal.
func TestDefaultDir_Precedencia(t *testing.T) {
	for _, tc := range []struct {
		name               string
		pubkit, jwlib, xdg string
		want               func(string) string
	}{{
		name:   "JWPUBKIT_HOME manda sobre todo lo demás",
		pubkit: "/tmp/a", jwlib: "/tmp/b", xdg: "/tmp/c",
		want: func(string) string { return "/tmp/a" },
	}, {
		name:  "JWLIB_HOME sigue valiendo cuando no está el nuevo",
		jwlib: "/tmp/b", xdg: "/tmp/c",
		want: func(string) string { return "/tmp/b" },
	}, {
		name: "XDG_DATA_HOME cuelga la biblioteca de su propio árbol",
		xdg:  "/tmp/c",
		want: func(string) string { return filepath.Join("/tmp/c", "jwlib") },
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JWPUBKIT_HOME", tc.pubkit)
			t.Setenv("JWLIB_HOME", tc.jwlib)
			t.Setenv("XDG_DATA_HOME", tc.xdg)
			if got := DefaultDir(); got != tc.want(got) {
				t.Errorf("DefaultDir() = %q, quería %q", got, tc.want(got))
			}
		})
	}
}

// Sin ninguna variable, la ruta histórica: es donde están las bibliotecas que ya existen.
func TestDefaultDir_SinVariables(t *testing.T) {
	t.Setenv("JWPUBKIT_HOME", "")
	t.Setenv("JWLIB_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	got := DefaultDir()
	if want := filepath.Join(".local", "share", "jwlib"); filepath.Base(got) != "jwlib" ||
		!filepath.IsAbs(got) || got[len(got)-len(want):] != want {
		t.Errorf("DefaultDir() = %q, quería una ruta absoluta que acabe en %q", got, want)
	}
}
