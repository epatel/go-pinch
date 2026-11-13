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

	"github.com/peterhellberg/go-pinch/internal/ziptest"
)

// zipFiles is the fixture content, in the order it is written to the archive.
var zipFiles = []ziptest.File{
	{Name: "data.json", Data: []byte(`{"gem":"pinch","authors":["Peter Hellberg","Edward Patel"]}`), Method: zip.Deflate},
	{Name: "images/note.txt", Data: []byte("a pine cone"), Method: zip.Deflate},
}

// newZipServer serves zipFiles at /test.zip with full Range support.
func newZipServer(t *testing.T) string {
	t.Helper()
	return ziptest.NewServer(t, zipFiles...)
}

func TestGet(t *testing.T) {
	url := newZipServer(t) + "/test.zip"

	for _, f := range zipFiles {
		got, err := Get(t.Context(), url, f.Name)
		if err != nil {
			t.Fatalf("Get(%q): %v", f.Name, err)
		}
		if !bytes.Equal(got, f.Data) {
			t.Errorf("Get(%q) = %q, want %q", f.Name, got, f.Data)
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
	if start, end, ok := parseRange(r.Header.Get("Range"), int64(len(s.content))); ok {
		s.mark(start, end)
	}
	ziptest.ServeContent(w, r, s.content)
}

// parseRange parses a single "bytes=start-end" Range header and clamps it to
// [0, size). It reports false for anything else, including suffix and
// open-ended ranges, which this test never sends.
func parseRange(header string, size int64) (start, end int64, ok bool) {
	spec, ok := strings.CutPrefix(header, "bytes=")
	if !ok {
		return 0, 0, false
	}
	lo, hi, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, false
	}
	start, err1 := strconv.ParseInt(lo, 10, 64)
	end, err2 := strconv.ParseInt(hi, 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return max(start, 0), min(end, size-1), true
}

// mark records bytes start..end (inclusive) as served.
func (s *rangeTrackingServer) mark(start, end int64) {
	for i := start; i <= end; i++ {
		if !s.served[i] {
			s.served[i] = true
			s.bytes++
		}
	}
}

// servedIn counts the served bytes in [from, to) and returns the first served
// offset, or -1 if none was served.
func (s *rangeTrackingServer) servedIn(from, to int64) (count int, first int64) {
	first = -1
	for i := from; i < to; i++ {
		if !s.served[i] {
			continue
		}
		count++
		if first < 0 {
			first = i
		}
	}
	return count, first
}

// patternedBlob returns n bytes with a non-repeating-looking pattern, so the
// blob can be located verbatim inside an archive.
func patternedBlob(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 31)
	}
	return b
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
	big := patternedBlob(1 << 20)
	small := []byte("the one true pine cone")

	content := ziptest.Archive(t,
		ziptest.File{Name: "big.bin", Data: big, Method: zip.Store},
		ziptest.File{Name: "small.txt", Data: small, Method: zip.Deflate},
	)
	bigOffset := int64(bytes.Index(content, big)) // start of the stored big.bin body
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

	bodyServed, firstServed := srv.servedIn(bigOffset, bigOffset+int64(len(big)))
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
		want[i] = f.Name
	}
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}
