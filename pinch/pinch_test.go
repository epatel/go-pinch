package pinch

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
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

// rangeTrackingServer serves content and records the union of byte offsets it
// actually transmits, so a test can prove only part of the archive was read.
type rangeTrackingServer struct {
	content []byte
	served  []bool // served[i] == true once byte i has been sent to a client
	bytes   int64  // total distinct bytes transmitted across all responses
}

func (s *rangeTrackingServer) handler(w http.ResponseWriter, r *http.Request) {
	// http.ServeContent parses the Range header for us; re-parse it here to
	// mark which offsets get sent.
	if spec, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok {
		if lo, hi, ok := strings.Cut(spec, "-"); ok {
			start, err1 := strconv.ParseInt(lo, 10, 64)
			end, err2 := strconv.ParseInt(hi, 10, 64)
			if err1 == nil && err2 == nil {
				if end >= int64(len(s.content)) {
					end = int64(len(s.content)) - 1
				}
				for i := start; i <= end && i >= 0 && i < int64(len(s.content)); i++ {
					if !s.served[i] {
						s.served[i] = true
						s.bytes++
					}
				}
			}
		}
	}
	http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(s.content))
}

// TestOnlyPartialDownload is a regression guard against accidentally fetching
// the whole archive: it builds an archive with a large stored file and a small
// file, fetches only the small file, and asserts the large file's body is never
// transmitted. The lone exception is archive/zip's end-of-central-directory
// scan, which reads the last 1 KiB of the file to find the EOCD signature and
// so unavoidably laps over the tail of the preceding file's body.
func TestOnlyPartialDownload(t *testing.T) {
	// A 1 MiB blob, stored (not deflated) so its bytes appear verbatim in the
	// archive as a large, contiguous, identifiable region.
	big := make([]byte, 1<<20)
	for i := range big {
		big[i] = byte(i * 31)
	}
	small := []byte("the one true pine cone")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	bw, err := zw.CreateHeader(&zip.FileHeader{Name: "big.bin", Method: zip.Store})
	if err != nil {
		t.Fatalf("CreateHeader big.bin: %v", err)
	}
	if _, err := bw.Write(big); err != nil {
		t.Fatalf("write big.bin: %v", err)
	}

	sw, err := zw.Create("small.txt")
	if err != nil {
		t.Fatalf("Create small.txt: %v", err)
	}
	if _, err := sw.Write(small); err != nil {
		t.Fatalf("write small.txt: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}

	content := buf.Bytes()
	bigOffset := bytes.Index(content, big) // start of the stored big.bin body
	if bigOffset < 0 {
		t.Fatal("could not locate big.bin body in archive")
	}

	srv := &rangeTrackingServer{content: content, served: make([]bool, len(content))}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	t.Cleanup(ts.Close)

	got, err := Get(t.Context(), ts.URL+"/test.zip", "small.txt")
	if err != nil {
		t.Fatalf("Get(small.txt): %v", err)
	}
	if !bytes.Equal(got, small) {
		t.Fatalf("Get(small.txt) = %q, want %q", got, small)
	}

	// The body of big.bin BEFORE the trailing EOCD scan window must never be
	// transmitted: that would mean we downloaded the file we never asked for.
	const eocdScanWindow = 1024
	scanFloor := int64(len(content)) - eocdScanWindow

	bodyServed, firstServed := 0, int64(-1)
	for i := int64(bigOffset); i < int64(bigOffset)+int64(len(big)); i++ {
		if srv.served[i] {
			bodyServed++
			if firstServed < 0 {
				firstServed = i
			}
		}
	}
	if firstServed >= 0 && firstServed < scanFloor {
		t.Errorf("big.bin body fetched at offset %d, before the EOCD scan floor %d",
			firstServed, scanFloor)
	}

	t.Logf("archive size:        %d bytes", len(content))
	t.Logf("total transmitted:   %d bytes (%.3f%%)", srv.bytes, 100*float64(srv.bytes)/float64(len(content)))
	t.Logf("big.bin body served: %d of %d bytes, within the trailing %d-byte EOCD scan window",
		bodyServed, len(big), eocdScanWindow)

	// The whole point: far less than the big payload crosses the wire.
	if srv.bytes >= int64(len(big)) {
		t.Errorf("transmitted %d bytes, expected far less than the %d-byte payload",
			srv.bytes, len(big))
	}
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
