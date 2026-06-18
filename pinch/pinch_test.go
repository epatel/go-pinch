package pinch

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

// zipFiles is the fixture content, in the order it is written to the archive.
var zipFiles = []struct {
	name string
	data []byte
}{
	{"data.json", []byte(`{"gem":"pinch","authors":["Peter Hellberg","Edward Patel"]}`)},
	{"images/note.txt", []byte("a pine cone")},
}

// newZipServer builds an in-memory ZIP archive from zipFiles and serves it at
// /test.zip with full Range support via http.ServeContent.
func newZipServer(t *testing.T) string {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zipFiles {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", f.name, err)
		}
		if _, err := w.Write(f.data); err != nil {
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

func TestGet(t *testing.T) {
	url := newZipServer(t) + "/test.zip"

	for _, f := range zipFiles {
		got, err := Get(t.Context(), url, f.name)
		if err != nil {
			t.Fatalf("Get(%q): %v", f.name, err)
		}
		if !bytes.Equal(got, f.data) {
			t.Errorf("Get(%q) = %q, want %q", f.name, got, f.data)
		}
	}
}

func TestGetMissingFile(t *testing.T) {
	url := newZipServer(t) + "/test.zip"

	_, err := Get(t.Context(), url, "missing.json")
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("Get missing file: got %v, want ErrFileNotFound", err)
	}
}

func TestGetMissingArchive(t *testing.T) {
	url := newZipServer(t) + "/missing.zip"

	_, err := Get(t.Context(), url, "data.json")

	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("Get missing archive: got %v, want *StatusError", err)
	}
	if se.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", se.StatusCode, http.StatusNotFound)
	}
}

func TestNoRangeSupport(t *testing.T) {
	// A server that ignores the Range header and always replies 200 OK.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not a zip"))
	}))
	t.Cleanup(srv.Close)

	_, err := Open(t.Context(), srv.URL)
	if err == nil {
		t.Fatal("Open succeeded, want range-support error")
	}
	if got, want := err.Error(), "server does not support range requests"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestWithClient(t *testing.T) {
	url := newZipServer(t) + "/test.zip"

	rt := &countingTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: rt}

	if _, err := Get(t.Context(), url, "data.json", WithClient(client)); err != nil {
		t.Fatalf("Get with custom client: %v", err)
	}
	if rt.calls == 0 {
		t.Error("custom client was not used")
	}

	// A nil client must be ignored, leaving the default in place.
	if _, err := Get(t.Context(), url, "data.json", WithClient(nil)); err != nil {
		t.Fatalf("Get with nil client: %v", err)
	}
}

type countingTransport struct {
	base  http.RoundTripper
	calls int
}

func (t *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls++
	return t.base.RoundTrip(r)
}

func TestList(t *testing.T) {
	url := newZipServer(t) + "/test.zip"

	got, err := List(t.Context(), url)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := make([]string, len(zipFiles))
	for i, f := range zipFiles {
		want[i] = f.name
	}
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}
