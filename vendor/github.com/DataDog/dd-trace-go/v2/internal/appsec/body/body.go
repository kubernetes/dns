// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package body

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal/appsec/body/json"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/go-libddwaf/v4"
)

// IsBodySupported checks if the body should be analyzed based on content type
func IsBodySupported(contentType string) bool {
	if contentType == "" {
		return false
	}

	parsedCT, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Debug("failed to parse content type %q: %s", contentType, err.Error())
		return false
	}

	return strings.HasSuffix(parsedCT, "json")
}

// NewEncodable creates a new libddwaf.Encodable from an io.ReadCloser with a size limit
// It reads up to 'limit' bytes from the reader and returns an error if reading fails
// If the content is larger than 'limit', it will be truncated and the returned Encodable will have the truncated flag set to true
// The given reader is not closed by this function and is replaced by a new io.ReadCloser that reads the all the data as if
// this function had not been called.
func NewEncodable(contentType string, reader *io.ReadCloser, limit int) (libddwaf.Encodable, error) {
	if !IsBodySupported(contentType) {
		return nil, nil
	}

	if reader == nil || *reader == nil {
		return nil, errors.New("reader is nil")
	}

	limitedReader := io.LimitedReader{
		R: *reader,
		N: int64(limit),
	}

	data, err := io.ReadAll(&limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	var newReader io.Reader = bytes.NewReader(data)

	truncated := false
	if len(data) >= limit {
		data = data[:limit]
		newReader = io.MultiReader(newReader, *reader)
		truncated = true
	}

	*reader = &readerAndCloser{
		Reader: newReader,
		Closer: *reader,
	}

	return NewEncodableFromData(contentType, data, truncated)
}

type readerAndCloser struct {
	io.Reader
	io.Closer
}

func NewEncodableFromData(contentType string, data []byte, truncated bool) (libddwaf.Encodable, error) {
	parsedCT, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Debug("failed to parse content type, no body parsing will be performed on data type: %s", err.Error())
		return nil, err
	}

	switch {
	// Handle cases like:
	// * application/json: https://www.iana.org/assignments/media-types/application/json
	// * application/vnd.api+json: https://jsonapi.org/
	// * text/json: https://mimetype.io/text/json
	case strings.HasSuffix(parsedCT, "json"):
		return json.NewEncodableFromData(data, truncated), nil
	}

	return nil, fmt.Errorf("unsupported content type: %s", contentType)
}
