// Package ziptest builds in-memory ZIP archives and serves them over HTTP with
// Range support, for tests.
package ziptest

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// File is one entry in a test archive. Method defaults to zip.Store (0); set
// it to zip.Deflate to compress the entry.
type File struct {
	Name   string
	Data   []byte
	Method uint16
}

// Archive returns a ZIP archive containing files, in order.
func Archive(t testing.TB, files ...File) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method})
		if err != nil {
			t.Fatalf("zip.CreateHeader(%q): %v", f.Name, err)
		}
		if _, err := w.Write(f.Data); err != nil {
			t.Fatalf("write %q: %v", f.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

// ServeContent serves content via http.ServeContent, which handles Range
// requests.
func ServeContent(w http.ResponseWriter, r *http.Request, content []byte) {
	http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(content))
}

// NewServer serves an archive of files at /test.zip and returns the server's
// base URL. The server is closed when the test ends.
func NewServer(t testing.TB, files ...File) string {
	t.Helper()

	content := Archive(t, files...)
	mux := http.NewServeMux()
	mux.HandleFunc("/test.zip", func(w http.ResponseWriter, r *http.Request) {
		ServeContent(w, r, content)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}
