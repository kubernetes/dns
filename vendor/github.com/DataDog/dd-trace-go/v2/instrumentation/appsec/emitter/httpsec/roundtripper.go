// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package httpsec

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/appsec/events"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

var badInputContextOnce sync.Once

type (
	RoundTripOperation struct {
		dyngo.Operation
		HandlerOp *HandlerOperation

		analyseBody bool
	}

	// RoundTripOperationArgs is the round trip operation arguments.
	RoundTripOperationArgs struct {
		// URL corresponds to the address `server.io.net.url`.
		URL     string
		Method  string
		Headers map[string][]string
		Body    *io.ReadCloser
	}

	// RoundTripOperationRes is the round trip operation results.
	RoundTripOperationRes struct {
		StatusCode int
		Headers    map[string][]string
		Body       *io.ReadCloser
	}
)

func (r *RoundTripOperation) SetAnalyseBody() {
	r.analyseBody = true
}

func (r *RoundTripOperation) AnalyseBody() bool {
	return r.analyseBody
}

func (RoundTripOperationArgs) IsArgOf(*RoundTripOperation)   {}
func (RoundTripOperationRes) IsResultOf(*RoundTripOperation) {}

// ProtectRoundTrip starts a round trip operation in the given context.
// If the context does not contain a parent operation, it returns nil.
// If the request is blocked by the WAF, it returns a [events.BlockingSecurityEvent] error.
// The returned function must be called before the span is finished, with the response and error of the round trip.
// If an error is returned, the returned function must not be called.
func ProtectRoundTrip(ctx context.Context, req *http.Request) (func(*http.Response), error) {
	opArgs := RoundTripOperationArgs{
		URL:     req.URL.String(),
		Method:  req.Method,
		Headers: req.Header,
		Body:    &req.Body,
	}

	handlerOp, ok := dyngo.FindOperation[HandlerOperation](ctx)
	if !ok { // No parent operation => we can't monitor the request
		badInputContextOnce.Do(func() {
			log.Debug("appsec: outgoing http request monitoring ignored: could not find the handler " +
				"instrumentation metadata in the request context: the request handler is not being monitored by a " +
				"middleware function or the incoming request context has not be forwarded correctly to the roundtripper")
		})
		return nil, nil
	}

	op := &RoundTripOperation{
		Operation: dyngo.NewOperation(handlerOp),
		HandlerOp: handlerOp,
	}

	var err *events.BlockingSecurityEvent
	// TODO: move the data listener as a setup function of httpsec.StartRoundTripperOperation(ars, <setup>)
	dyngo.OnData(op, func(e *events.BlockingSecurityEvent) {
		err = e
	})

	dyngo.StartOperation(op, opArgs)

	if err != nil {
		log.Debug("appsec: outgoing http request blocked by the WAF on URL: %s", req.URL.String())
		return nil, err
	}

	return func(response *http.Response) {
		var resArgs RoundTripOperationRes
		if response != nil {
			resArgs = RoundTripOperationRes{
				StatusCode: response.StatusCode,
				Headers:    response.Header,
				Body:       &response.Body,
			}
		}
		dyngo.FinishOperation(op, resArgs)
	}, nil
}
