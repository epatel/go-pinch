# Go Pinch

[![Go Reference](https://pkg.go.dev/badge/github.com/peterhellberg/go-pinch/pinch.svg)](https://pkg.go.dev/github.com/peterhellberg/go-pinch/pinch)
[![License MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg?style=flat)](https://github.com/peterhellberg/go-pinch#license-mit)

## Retrieve a file from inside a zip archive, over the network!

Pinch makes it possible to download a specific file from within
a ZIP archive over HTTP, using nothing but the Go standard
library ([net/http](https://pkg.go.dev/net/http) and
[archive/zip](https://pkg.go.dev/archive/zip)). It serves an
`io.ReaderAt` backed by HTTP Range requests to `archive/zip`,
so only the bytes it actually needs are fetched.

Earlier versions were written in [Objective-C](https://github.com/epatel/pinch-objc), [Ruby](https://github.com/peterhellberg/pinch) and [Java](https://github.com/carlbenson/Pinch)

*STATUS:* Working, but not in any known use :)

## Installation

```bash
go install github.com/peterhellberg/go-pinch@latest
```

## Usage

List the files in a remote archive by passing only a URL:

```bash
$ go-pinch https://peterhellberg.github.io/pinch/test.zip
data.json
images/pine_cone.jpg
```

Pinch a single file out of it by also passing a file name:

```bash
$ go-pinch https://peterhellberg.github.io/pinch/test.zip data.json > data.json
```

> [!TIP]
> You can run the binary without installing it first using `go run`:
>
> ```bash
> go run github.com/peterhellberg/go-pinch@latest https://peterhellberg.github.io/pinch/test.zip
> ```

Or from Go directly:

```go
package main

import (
	"context"
	"os"

	"github.com/peterhellberg/go-pinch/pinch"
)

func main() {
	data, err := pinch.Get(context.Background(),
		"https://peterhellberg.github.io/pinch/test.zip", "data.json")
	if err != nil {
		panic(err)
	}

	os.Stdout.Write(data)
}
```

List the files in an archive:

```go
names, err := pinch.List(ctx, "https://peterhellberg.github.io/pinch/test.zip")
```

Use a custom `*http.Client`, e.g. to add authentication or a timeout:

```go
client := &http.Client{Timeout: 30 * time.Second}

data, err := pinch.Get(ctx, url, "data.json", pinch.WithClient(client))
```

For full random access — iterating entries, opening files lazily — call
`pinch.Open`, which returns an [`*archive/zip.Reader`](https://pkg.go.dev/archive/zip#Reader):

```go
zr, err := pinch.Open(ctx, url)
if err != nil {
	panic(err)
}

for _, f := range zr.File {
	fmt.Println(f.Name)
}
```

## License (MIT)

Copyright (c) 2013-2026 [Peter Hellberg](http://c7.se/), [Edward Patel](http://memention.com/)

> Permission is hereby granted, free of charge, to any person obtaining
> a copy of this software and associated documentation files (the
> "Software"), to deal in the Software without restriction, including
> without limitation the rights to use, copy, modify, merge, publish,
> distribute, sublicense, and/or sell copies of the Software, and to
> permit persons to whom the Software is furnished to do so, subject to
> the following conditions:

> The above copyright notice and this permission notice shall be
> included in all copies or substantial portions of the Software.

> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
> EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
> MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
> NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
> LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
> OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
> WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
