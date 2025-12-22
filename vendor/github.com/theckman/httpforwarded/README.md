# httpforwarded
[![TravisCI Build Status](https://img.shields.io/travis/theckman/httpforwarded/master.svg)](https://travis-ci.org/theckman/httpforwarded)
[![License](https://img.shields.io/badge/license-BSD--style_3--clause-brightgreen.svg?style=flat)](https://github.com/theckman/httpforwarded/blob/master/LICENSE)
[![GoDoc](https://img.shields.io/badge/GoDoc-httpforwarded-blue.svg)](https://godoc.org/github.com/theckman/httpforwarded)

The `httpforwarded` go package provides utility functions for working with the
`Forwarded` HTTP header as defined in [RFC-7239](https://tools.ietf.org/html/rfc7239).
This header is proposed to replace the `X-Forwarded-For` and `X-Forwarded-Proto`
headers, amongst others.

This package was heavily inspired by the `mime` package in the standard library,
more specifically the [ParseMediaType()](https://golang.org/pkg/mime/#ParseMediaType)
function.

## License
This package copies some functions, without modification, from the Go standard
library. As such, the entirety of this package is released under the same
permissive BSD-style license as the Go language itself. Please see the contents
of the [LICENSE](https://github.com/theckman/httpforwarded/blob/master/LICENSE)
file for the full details of the license.

## Installing
To install this package for consumption, you can run the following:

```
go get -u github.com/theckman/httpforwarded
```

If you would like to also work on the development of `httpforwarded`, you can
also install the testing dependencies:

```
go get -t -u github.com/theckman/httpforwarded
```

## Usage
```Go
// var req *http.Request

headerValues := req.Header[http.CanonicalHeaderKey("forwarded")]

params, _ := httpforwarded.Parse(headerValues)

// you can then do something like this to get the first "for" param:
fmt.Printf("origin %s", params["for"][0])
```

For more information on using the package, please refer to the
[GoDoc](https://godoc.org/github.com/theckman/httpforwarded) page.
