package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantURL  string
		wantFile string
		wantErr  bool
	}{
		{"no args", []string{"go-pinch"}, "", "", true},
		{"too many args", []string{"go-pinch", "http://x/z.zip", "a", "b"}, "", "", true},
		{"invalid url", []string{"go-pinch", "foo"}, "", "", true},
		{"ftp scheme", []string{"go-pinch", "ftp://x/z.zip"}, "", "", true},
		{"url only", []string{"go-pinch", "http://x/z.zip"}, "http://x/z.zip", "", false},
		{"url and file", []string{"go-pinch", "https://x/z.zip", "a.json"}, "https://x/z.zip", "a.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotFile, err := parseArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs err = %v, wantErr %v", err, tt.wantErr)
			}
			if gotURL != tt.wantURL || gotFile != tt.wantFile {
				t.Errorf("parseArgs = (%q, %q), want (%q, %q)", gotURL, gotFile, tt.wantURL, tt.wantFile)
			}
		})
	}
}

func TestRun(t *testing.T) {
	url := newZipServer(t) + "/test.zip"

	t.Run("list", func(t *testing.T) {
		out := captureStdout(t, func(f *os.File) error {
			return run(t.Context(), f, url, "")
		})
		for _, want := range []string{"data.json", "images/note.txt"} {
			if !strings.Contains(out, want) {
				t.Errorf("list output %q does not contain %q", out, want)
			}
		}
	})

	t.Run("get", func(t *testing.T) {
		out := captureStdout(t, func(f *os.File) error {
			return run(t.Context(), f, url, "data.json")
		})
		if !strings.Contains(out, `"gem":"pinch"`) {
			t.Errorf("get output = %q", out)
		}
	})
}

// newZipServer serves a small in-memory ZIP archive at /test.zip with Range
// support.
func newZipServer(t *testing.T) string {
	t.Helper()

	files := []struct{ name, data string }{
		{"data.json", `{"gem":"pinch","authors":["Peter Hellberg"]}`},
		{"images/note.txt", "a pine cone"},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", f.name, err)
		}
		if _, err := w.Write([]byte(f.data)); err != nil {
			t.Fatalf("write %q: %v", f.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	content := buf.Bytes()

	mux := http.NewServeMux()
	mux.HandleFunc("/test.zip", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(content))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL
}

// captureStdout runs fn with a temp file and returns what it wrote.
func captureStdout(t *testing.T, fn func(*os.File) error) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := fn(f); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}
