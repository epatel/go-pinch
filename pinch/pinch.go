// Package pinch retrieves individual files from inside a remote ZIP archive
// over HTTP, without downloading the whole archive.
//
// It works by serving an [io.ReaderAt] backed by HTTP Range requests to
// [archive/zip], so only the bytes that archive/zip actually reads — the end
// record, the central directory, and the requested file — are fetched.
package pinch

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

// ErrFileNotFound is returned by [Get] when the named file is not present in
// the archive.
var ErrFileNotFound = errors.New("file not found in archive")

// StatusError is returned when the server responds with an unexpected HTTP
// status, such as 404 for a missing archive.
type StatusError struct {
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string { return e.Status }

// An Option configures optional behaviour for [Open], [Get] and [List].
type Option func(*config)

type config struct {
	client *http.Client
}

func newConfig(opts []Option) config {
	cfg := config{client: http.DefaultClient}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithClient sets the [http.Client] used to fetch archive data. A nil client
// is ignored, leaving the default in place. Request cancellation is driven by
// the context passed to [Open], [Get] and [List], so a client timeout is
// usually unnecessary.
func WithClient(c *http.Client) Option {
	return func(cfg *config) {
		if c != nil {
			cfg.client = c
		}
	}
}

// Open returns a [zip.Reader] for the ZIP archive at url. The reader fetches
// archive data lazily through HTTP Range requests; the returned reader is only
// valid while ctx is not cancelled.
func Open(ctx context.Context, url string, opts ...Option) (*zip.Reader, error) {
	cfg := newConfig(opts)

	size, err := remoteSize(ctx, cfg.client, url)
	if err != nil {
		return nil, err
	}

	return zip.NewReader(&readerAt{ctx: ctx, client: cfg.client, url: url}, size)
}

// Get retrieves the contents of the named file from the ZIP archive at url. It
// returns [ErrFileNotFound] if the archive does not contain name.
func Get(ctx context.Context, url, name string, opts ...Option) ([]byte, error) {
	zr, err := Open(ctx, url, opts...)
	if err != nil {
		return nil, err
	}

	f, err := zr.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

// List returns the names of the files (excluding directories) in the ZIP
// archive at url.
func List(ctx context.Context, url string, opts ...Option) ([]string, error) {
	zr, err := Open(ctx, url, opts...)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		names = append(names, f.Name)
	}

	return names, nil
}

// readerAt implements [io.ReaderAt] by issuing an HTTP Range request per read.
type readerAt struct {
	ctx    context.Context
	client *http.Client
	url    string
}

func (r *readerAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1))

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, &StatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	n, err := io.ReadFull(resp.Body, p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		// A short final read means we asked past the end of the archive.
		err = io.EOF
	}

	return n, err
}

// remoteSize determines the total size of the resource at url, preferring the
// total reported in a Content-Range header from a tiny Range request.
func remoteSize(ctx context.Context, client *http.Client, url string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusPartialContent:
		// Content-Range: bytes 0-0/12345
		cr := resp.Header.Get("Content-Range")
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			if size, err := strconv.ParseInt(cr[i+1:], 10, 64); err == nil {
				return size, nil
			}
		}
		return 0, errors.New("could not parse Content-Range header")
	case http.StatusOK:
		// Server ignored the Range header and sent the whole body, so it
		// cannot serve the partial reads the rest of the package relies on.
		return 0, errors.New("server does not support range requests")
	default:
		return 0, &StatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
}
