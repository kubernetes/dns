// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/internal/tracerstats"
	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/version"

	"github.com/tinylib/msgp/msgp"
)

const (
	// headerComputedTopLevel specifies that the client has marked top-level spans, when set.
	// Any non-empty value will mean 'yes'.
	headerComputedTopLevel = "Datadog-Client-Computed-Top-Level"
)

func defaultDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
}

func defaultHTTPClient(timeout time.Duration, disableKeepAlives bool) *http.Client {
	if timeout == 0 {
		timeout = defaultHTTPTimeout
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           defaultDialer(timeout).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     disableKeepAlives,
		},
		Timeout: timeout,
	}
}

const (
	defaultHostname          = "localhost"
	defaultPort              = "8126"
	defaultAddress           = defaultHostname + ":" + defaultPort
	defaultURL               = "http://" + defaultAddress
	defaultHTTPTimeout       = 10 * time.Second              // defines the current timeout before giving up with the send process
	traceCountHeader         = "X-Datadog-Trace-Count"       // header containing the number of traces in the payload
	obfuscationVersionHeader = "Datadog-Obfuscation-Version" // header containing the version of obfuscation used, if any

	tracesAPIPath   = "/v0.4/traces"
	tracesAPIPathV1 = "/v1.0/traces"
	statsAPIPath    = "/v0.6/stats"
)

// transport is an interface for communicating data to the agent.
type transport interface {
	// send sends the payload p to the agent using the transport set up.
	// It returns a non-nil response body when no error occurred.
	send(p payload) (body io.ReadCloser, err error)
	// sendStats sends the given stats payload to the agent.
	// tracerObfuscationVersion is the version of obfuscation applied (0 if none was applied)
	sendStats(s *pb.ClientStatsPayload, tracerObfuscationVersion int) error
	// endpoint returns the URL to which the transport will send traces.
	endpoint() string
}

type httpTransport struct {
	traceURL string            // the delivery URL for traces
	statsURL string            // the delivery URL for stats
	client   *http.Client      // the HTTP client used in the POST
	headers  map[string]string // the Transport headers
}

// newTransport returns a new Transport implementation that sends traces to a
// trace agent at the given url, using a given *http.Client.
//
// In general, using this method is only necessary if you have a trace agent
// running on a non-default port, if it's located on another machine, or when
// otherwise needing to customize the transport layer, for instance when using
// a unix domain socket.
func newHTTPTransport(url string, client *http.Client) *httpTransport {
	// initialize the default EncoderPool with Encoder headers
	defaultHeaders := map[string]string{
		"Datadog-Meta-Lang":             "go",
		"Datadog-Meta-Lang-Version":     strings.TrimPrefix(runtime.Version(), "go"),
		"Datadog-Meta-Lang-Interpreter": runtime.Compiler + "-" + runtime.GOARCH + "-" + runtime.GOOS,
		"Datadog-Meta-Tracer-Version":   version.Tag,
		"Content-Type":                  "application/msgpack",
	}
	if cid := internal.ContainerID(); cid != "" {
		defaultHeaders["Datadog-Container-ID"] = cid
	}
	if eid := internal.EntityID(); eid != "" {
		defaultHeaders["Datadog-Entity-ID"] = eid
	}
	if extEnv := internal.ExternalEnvironment(); extEnv != "" {
		defaultHeaders["Datadog-External-Env"] = extEnv
	}
	return &httpTransport{
		traceURL: fmt.Sprintf("%s%s", url, tracesAPIPath),
		statsURL: fmt.Sprintf("%s%s", url, statsAPIPath),
		client:   client,
		headers:  defaultHeaders,
	}
}

func (t *httpTransport) sendStats(p *pb.ClientStatsPayload, tracerObfuscationVersion int) error {
	var buf bytes.Buffer
	if err := msgp.Encode(&buf, p); err != nil {
		return err
	}
	req, err := http.NewRequest("POST", t.statsURL, &buf)
	if err != nil {
		return err
	}
	for header, value := range t.headers {
		req.Header.Set(header, value)
	}
	if tracerObfuscationVersion > 0 {
		req.Header.Set(obfuscationVersionHeader, strconv.Itoa(tracerObfuscationVersion))
	}
	resp, err := t.client.Do(req)
	if err != nil {
		reportAPIErrorsMetric(resp, err, statsAPIPath)
		return err
	}
	defer resp.Body.Close()
	if code := resp.StatusCode; code >= 400 {
		reportAPIErrorsMetric(resp, err, statsAPIPath)
		// error, check the body for context information and
		// return a nice error.
		msg := make([]byte, 1000)
		n, _ := resp.Body.Read(msg)
		resp.Body.Close()
		txt := http.StatusText(code)
		if n > 0 {
			return fmt.Errorf("%s (Status: %s)", msg[:n], txt)
		}
		return fmt.Errorf("%s", txt)
	}
	return nil
}

func (t *httpTransport) send(p payload) (body io.ReadCloser, err error) {
	req, err := http.NewRequest("POST", t.traceURL, p)
	if err != nil {
		return nil, fmt.Errorf("cannot create http request: %s", err)
	}
	stats := p.stats()
	req.ContentLength = int64(stats.size)
	for header, value := range t.headers {
		req.Header.Set(header, value)
	}
	req.Header.Set(traceCountHeader, strconv.Itoa(stats.itemCount))
	req.Header.Set(headerComputedTopLevel, "yes")
	if t := getGlobalTracer(); t != nil {
		tc := t.TracerConf()
		if tc.TracingAsTransport || tc.CanComputeStats {
			// tracingAsTransport uses this header to disable the trace agent's stats computation
			// while making canComputeStats() always false to also disable client stats computation.
			req.Header.Set("Datadog-Client-Computed-Stats", "yes")
		}
		droppedTraces := int(tracerstats.Count(tracerstats.AgentDroppedP0Traces))
		partialTraces := int(tracerstats.Count(tracerstats.PartialTraces))
		droppedSpans := int(tracerstats.Count(tracerstats.AgentDroppedP0Spans))
		if tt, ok := t.(*tracer); ok {
			if stats := tt.statsd; stats != nil {
				stats.Count("datadog.tracer.dropped_p0_traces", int64(droppedTraces),
					[]string{fmt.Sprintf("partial:%s", strconv.FormatBool(partialTraces > 0))}, 1)
				stats.Count("datadog.tracer.dropped_p0_spans", int64(droppedSpans), nil, 1)
			}
		}
		req.Header.Set("Datadog-Client-Dropped-P0-Traces", strconv.Itoa(droppedTraces))
		req.Header.Set("Datadog-Client-Dropped-P0-Spans", strconv.Itoa(droppedSpans))
	}
	response, err := t.client.Do(req)
	if err != nil {
		reportAPIErrorsMetric(response, err, tracesAPIPath)
		return nil, err
	}
	if code := response.StatusCode; code >= 400 {
		reportAPIErrorsMetric(response, err, tracesAPIPath)
		// error, check the body for context information and
		// return a nice error.
		msg := make([]byte, 1000)
		n, _ := response.Body.Read(msg)
		response.Body.Close()
		txt := http.StatusText(code)
		if n > 0 {
			return nil, fmt.Errorf("%s (Status: %s)", msg[:n], txt)
		}
		return nil, fmt.Errorf("%s", txt)
	}
	return response.Body, nil
}

func reportAPIErrorsMetric(response *http.Response, err error, endpoint string) {
	if t, ok := getGlobalTracer().(*tracer); ok {
		var reason string
		if err != nil {
			reason = "network_failure"
		}
		if response != nil {
			reason = fmt.Sprintf("server_response_%d", response.StatusCode)
		}
		tags := []string{"reason:" + reason, "endpoint:" + endpoint}
		t.statsd.Incr("datadog.tracer.api.errors", tags, 1)
	} else {
		return
	}
}

func (t *httpTransport) endpoint() string {
	return t.traceURL
}
