/*
Retrieve a file from inside a zip file, over the network!

Pinch makes it possible to download a specific file from within a ZIP file
over HTTP, using nothing but the Go standard library.

Earlier versions were written in Objective-C and Ruby.

# Installation

Just go get it:

	go get -u github.com/peterhellberg/go-pinch

# Usage

You get a list of files if you only pass a URL:

	go-pinch http://example/path/to.zip

In order to pinch one of the files in the ZIP:

	go-pinch http://example/path/to.zip file.json > file.json
*/
package main

import (
	"context"
	"errors"
	"fmt"
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

func run(ctx context.Context, out *os.File, rawURL, name string) error {
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
		return "", "", errors.New("Usage: go-pinch <url> [file]")
	}

	u, err := url.ParseRequestURI(args[1])
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", errors.New("Invalid URL (must be http or https)")
	}

	if len(args) == 3 {
		return args[1], args[2], nil
	}

	return args[1], "", nil
}
