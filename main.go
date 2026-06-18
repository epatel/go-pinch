/*
Retrieve a file from inside a zip archive, over the network!

Pinch downloads a specific file from within a ZIP archive over HTTP,
using nothing but the Go standard library.

Earlier versions were written in Objective-C and Ruby.

# Installation

	go install github.com/peterhellberg/go-pinch@latest

# Usage

List the files in a remote archive by passing only a URL:

	go-pinch https://example.com/path/to.zip

Pinch one of the files out of the archive:

	go-pinch https://example.com/path/to.zip file.json > file.json
*/
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"

	"github.com/peterhellberg/go-pinch/pinch"
)

func main() {
	log.SetFlags(0)

	rawURL, name, err := parseArgs(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	if err := run(context.Background(), os.Stdout, rawURL, name); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, out io.Writer, rawURL, name string) error {
	if name != "" {
		data, err := pinch.Get(ctx, rawURL, name)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	}

	names, err := pinch.List(ctx, rawURL)
	if err != nil {
		return err
	}
	for _, n := range names {
		fmt.Fprintln(out, n)
	}

	return nil
}

func parseArgs(args []string) (rawURL, name string, err error) {
	if len(args) < 2 || len(args) > 3 {
		return "", "", errors.New("usage: go-pinch <url> [file]")
	}

	u, err := url.ParseRequestURI(args[1])
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", errors.New("invalid URL (must be http or https)")
	}

	if len(args) == 3 {
		return args[1], args[2], nil
	}

	return args[1], "", nil
}
