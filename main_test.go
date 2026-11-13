package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/peterhellberg/go-pinch/internal/ziptest"
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
		var out bytes.Buffer
		if err := run(t.Context(), &out, url, ""); err != nil {
			t.Fatalf("run: %v", err)
		}
		for _, want := range []string{"data.json", "images/note.txt"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("list output %q does not contain %q", out.String(), want)
			}
		}
	})

	t.Run("get", func(t *testing.T) {
		var out bytes.Buffer
		if err := run(t.Context(), &out, url, "data.json"); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out.String(), `"gem":"pinch"`) {
			t.Errorf("get output = %q", out.String())
		}
	})
}

// newZipServer serves a small in-memory ZIP archive at /test.zip with Range
// support.
func newZipServer(t *testing.T) string {
	t.Helper()
	return ziptest.NewServer(t,
		ziptest.File{Name: "data.json", Data: []byte(`{"gem":"pinch","authors":["Peter Hellberg"]}`), Method: zip.Deflate},
		ziptest.File{Name: "images/note.txt", Data: []byte("a pine cone"), Method: zip.Deflate},
	)
}
