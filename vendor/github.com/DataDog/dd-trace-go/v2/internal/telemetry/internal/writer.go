// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/globalconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/hostname"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/osinfo"
	"github.com/DataDog/dd-trace-go/v2/internal/processtags"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
	"github.com/DataDog/dd-trace-go/v2/internal/version"
)

// We copy the transport to avoid using the default one, as it might be
// augmented with tracing and we don't want these calls to be recorded.
// See https://golang.org/pkg/net/http/#DefaultTransport .
var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	Timeout: 5 * time.Second,
}

func newBody(config TracerConfig, debugMode bool) *transport.Body {
	osHostname, err := os.Hostname()
	if err != nil {
		osHostname = hostname.Get()
	}

	if osHostname == "" {
		osHostname = "unknown" // hostname field is not allowed to be empty
	}

	return &transport.Body{
		APIVersion: "v2",
		RuntimeID:  globalconfig.RuntimeID(),
		Debug:      debugMode,
		Application: transport.Application{
			ServiceName:     config.Service,
			Env:             config.Env,
			ServiceVersion:  config.Version,
			TracerVersion:   version.Tag,
			LanguageName:    "go",
			LanguageVersion: runtime.Version(),
			ProcessTags:     processtags.GlobalTags().String(),
		},
		Host: transport.Host{
			Hostname:      osHostname,
			OS:            osinfo.OSName(),
			OSVersion:     osinfo.OSVersion(),
			Architecture:  osinfo.Architecture(),
			KernelName:    osinfo.KernelName(),
			KernelRelease: osinfo.KernelRelease(),
			KernelVersion: osinfo.KernelVersion(),
		},
	}
}

// Writer is an interface that allows to send telemetry data to any endpoint that implements the instrumentation telemetry v2 API.
// The telemetry data is sent as a JSON payload as described in the API documentation.
type Writer interface {
	// Flush does a synchronous call to the telemetry endpoint with the given payload. Thread-safe.
	// It returns a non-empty [EndpointRequestResult] slice and a nil error if the payload was sent successfully.
	// Otherwise, the error is a call to [errors.Join] on all errors that occurred.
	Flush(transport.Payload) ([]EndpointRequestResult, error)
}

// EndpointRequestResult is returned by the Flush method of the Writer interface.
type EndpointRequestResult struct {
	// Error is the error that occurred when sending the payload to the endpoint. This is nil if the payload was sent successfully.
	Error error
	// PayloadByteSize is the number of bytes that were sent to the endpoint, zero if the payload was not sent.
	PayloadByteSize int
	// CallDuration is the duration of the call to the endpoint if the call was successful
	CallDuration time.Duration
	// StatusCode is the status code of the response from the endpoint even if the call failed but only with an actual HTTP error
	StatusCode int
}

type writer struct {
	mu         sync.Mutex
	body       *transport.Body
	bodyMu     sync.Mutex
	httpClient *http.Client
	endpoints  []*http.Request
}

type WriterConfig struct {
	// TracerConfig is the configuration the tracer sent when the telemetry client was created (required)
	TracerConfig
	// Endpoints is a list of requests that will be used alongside the body of the telemetry data to create the requests to the telemetry endpoint (required to not be empty)
	// The writer will try each endpoint in order until it gets a 2XX HTTP response from the server
	Endpoints []*http.Request
	// HTTPClient is the http client that will be used to send the telemetry data (defaults to a copy of [http.DefaultClient])
	HTTPClient *http.Client
	// Debug is a flag that indicates whether the telemetry client is in debug mode (defaults to false)
	Debug bool
}

func NewWriter(config WriterConfig) (Writer, error) {
	if len(config.Endpoints) == 0 {
		return nil, fmt.Errorf("telemetry/writer: no endpoints provided")
	}

	if config.HTTPClient == nil {
		config.HTTPClient = defaultHTTPClient
	}

	// Don't allow the client to have a timeout higher than 5 seconds
	// This is to avoid blocking the client for too long in case of network issues
	if config.HTTPClient.Timeout > 5*time.Second {
		copyClient := *config.HTTPClient
		config.HTTPClient = &copyClient
		config.HTTPClient.Timeout = 5 * time.Second
	}

	body := newBody(config.TracerConfig, config.Debug)
	endpoints := make([]*http.Request, len(config.Endpoints))
	for i, endpoint := range config.Endpoints {
		endpoints[i] = preBakeRequest(body, endpoint)
	}

	return &writer{
		body:       body,
		httpClient: config.HTTPClient,
		endpoints:  endpoints,
	}, nil
}

// preBakeRequest adds all the *static* headers that we already know at the time of the creation of the writer.
// This is useful to avoid querying too many things at the time of the request.
// Headers necessary are described here:
// https://github.com/DataDog/instrumentation-telemetry-api-docs/blob/cf17b41a30fbf31d54e2cfbfc983875d58b02fe1/GeneratedDocumentation/ApiDocs/v2/overview.md#required-http-headers
func preBakeRequest(body *transport.Body, endpoint *http.Request) *http.Request {
	clonedEndpoint := endpoint.Clone(context.Background())
	if clonedEndpoint.Header == nil {
		clonedEndpoint.Header = make(http.Header, 11)
	}

	for key, val := range map[string]string{
		"Content-Type":               "application/json",
		"DD-Telemetry-API-Version":   body.APIVersion,
		"DD-Client-Library-Language": body.Application.LanguageName,
		"DD-Client-Library-Version":  body.Application.TracerVersion,
		"DD-Agent-Env":               body.Application.Env,
		"DD-Agent-Hostname":          body.Host.Hostname,
		"DD-Agent-Install-Id":        globalconfig.InstrumentationInstallID(),
		"DD-Agent-Install-Type":      globalconfig.InstrumentationInstallType(),
		"DD-Agent-Install-Time":      globalconfig.InstrumentationInstallTime(),
		"Datadog-Container-ID":       internal.ContainerID(),
		"Datadog-Entity-ID":          internal.EntityID(),
		// TODO: add support for Cloud provider/resource-type/resource-id headers in another PR and package
		// Described here: https://github.com/DataDog/instrumentation-telemetry-api-docs/blob/cf17b41a30fbf31d54e2cfbfc983875d58b02fe1/GeneratedDocumentation/ApiDocs/v2/overview.md#setting-the-serverless-telemetry-headers
	} {
		if val == "" {
			continue
		}
		clonedEndpoint.Header.Add(key, val)
	}

	if body.Debug {
		clonedEndpoint.Header.Add("DD-Telemetry-Debug-Enabled", "true")
	}

	return clonedEndpoint
}

// setPayloadToBody sets the payload to the body of the writer and misc fields that are necessary for the payload to be sent.
func (w *writer) setPayloadToBody(payload transport.Payload) {
	w.bodyMu.Lock()
	defer w.bodyMu.Unlock()
	w.body.SeqID++
	w.body.TracerTime = time.Now().Unix()
	w.body.RequestType = payload.RequestType()
	w.body.Payload = payload
}

// newRequest creates a new http.Request with the given payload and the necessary headers.
func (w *writer) newRequest(endpoint *http.Request, requestType transport.RequestType) *http.Request {
	request := endpoint.Clone(context.Background())
	request.Header.Set("DD-Telemetry-Request-Type", string(requestType))

	pipeReader, pipeWriter := io.Pipe()
	request.Body = pipeReader
	go func() {
		var err error
		defer func() {
			// This should normally never happen but since we are encoding arbitrary data in client configuration values payload we need to be careful.
			if panicValue := recover(); panicValue != nil {
				log.Error("telemetry/writer: panic while encoding payload!")
				if err == nil {
					panicErr, ok := panicValue.(error) // check if we can use the panic value as an error
					if ok {
						log.Error("telemetry/writer: panic while encoding payload: %v", panicErr.Error())
					}
					pipeWriter.CloseWithError(panicErr) // CloseWithError with nil as parameter is like Close()
				}
			}
		}()

		// If a previous endpoint is still trying to marshall the body, we need to wait for it to realize the pipe is closed and exit.
		w.bodyMu.Lock()
		defer w.bodyMu.Unlock()

		// No need to wait on this because the http client will close the pipeReader which will close the pipeWriter and finish the goroutine
		err = json.NewEncoder(pipeWriter).Encode(w.body)
		pipeWriter.CloseWithError(err)
	}()

	return request
}

// SumReaderCloser is a ReadCloser that wraps another ReadCloser and counts the number of bytes read.
type SumReaderCloser struct {
	io.ReadCloser
	n int
}

func (s *SumReaderCloser) Read(p []byte) (n int, err error) {
	n, err = s.ReadCloser.Read(p)
	s.n += n
	return
}

// WriterStatusCodeError is an error that is returned when the writer receives an unexpected status code from the server.
type WriterStatusCodeError struct {
	Status string
	Body   string
}

func (w *WriterStatusCodeError) Error() string {
	return fmt.Sprintf("unexpected status code: %q (received body: %d bytes)", w.Status, len(w.Body))
}

func (w *writer) Flush(payload transport.Payload) ([]EndpointRequestResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.setPayloadToBody(payload)
	requestType := payload.RequestType()

	var results []EndpointRequestResult
	for _, endpoint := range w.endpoints {
		var (
			request         = w.newRequest(endpoint, requestType)
			sumReaderCloser = &SumReaderCloser{ReadCloser: request.Body}
			now             = time.Now()
		)

		request.Body = sumReaderCloser
		response, err := w.httpClient.Do(request)
		if err != nil {
			results = append(results, EndpointRequestResult{Error: err})
			continue
		}

		// We only have a few endpoints, so we can afford to keep the response body stream open until we are done with it
		defer response.Body.Close()

		if response.StatusCode >= 300 || response.StatusCode < 200 {
			respBodyBytes, _ := io.ReadAll(io.LimitReader(response.Body, 256)) // maybe we can find an error reason in the response body
			results = append(results, EndpointRequestResult{Error: &WriterStatusCodeError{
				Status: response.Status,
				Body:   string(respBodyBytes),
			}, StatusCode: response.StatusCode})
			continue
		}

		results = append(results, EndpointRequestResult{
			PayloadByteSize: sumReaderCloser.n,
			CallDuration:    time.Since(now),
			StatusCode:      response.StatusCode,
		})

		// We succeeded, no need to try the other endpoints
		break
	}

	var err error
	if results[len(results)-1].Error != nil {
		var errs []error
		for _, result := range results {
			errs = append(errs, result.Error)
		}
		err = errors.Join(errs...)
	}

	return results, err
}

// RecordWriter is a Writer that stores the payloads in memory. Used for testing purposes
type RecordWriter struct {
	mu       sync.Mutex
	payloads []transport.Payload
}

func (w *RecordWriter) Flush(payload transport.Payload) ([]EndpointRequestResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payloads = append(w.payloads, payload)
	return []EndpointRequestResult{
		{
			PayloadByteSize: 1,
			CallDuration:    time.Nanosecond,
		},
	}, nil
}

func (w *RecordWriter) Payloads() []transport.Payload {
	w.mu.Lock()
	defer w.mu.Unlock()
	copyPayloads := make([]transport.Payload, len(w.payloads))
	copy(copyPayloads, w.payloads)
	return copyPayloads
}

var _ Writer = (*RecordWriter)(nil)
