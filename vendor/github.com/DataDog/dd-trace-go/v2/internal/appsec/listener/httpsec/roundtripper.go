// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package httpsec

import (
	"log/slog"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/httpsec"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/addresses"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/body"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/listener"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
)

type DownwardRequestFeature struct {
	analysisSampleRate               float64
	maxDownstreamRequestBodyAnalysis int

	// downstreamRequestAnalysis is the number of times a call to a downstream request monitoring function was made
	// we don't really care if it overflows, as it's just used for sampling
	downstreamRequestAnalysis atomic.Uint64
}

func (*DownwardRequestFeature) String() string {
	return "SSRF Protection & OWASP API10 Protection"
}

func (*DownwardRequestFeature) Stop() {}

func NewSSRFProtectionFeature(config *config.Config, rootOp dyngo.Operation) (listener.Feature, error) {
	if !config.RASP || !config.SupportedAddresses.AnyOf(
		addresses.ServerIONetURLAddr,
		addresses.ServerIONetRequestMethodAddr,
		addresses.ServerIONetRequestHeadersAddr,
		addresses.ServerIONetRequestBodyAddr,
		addresses.ServerIONetResponseStatusAddr,
		addresses.ServerIONetResponseHeadersAddr,
		addresses.ServerIONetResponseBodyAddr) {
		return nil, nil
	}

	feature := &DownwardRequestFeature{
		analysisSampleRate:               config.APISec.DownstreamRequestBodyAnalysisSampleRate,
		maxDownstreamRequestBodyAnalysis: config.APISec.MaxDownstreamRequestBodyAnalysis,
	}
	dyngo.On(rootOp, feature.OnStart)
	dyngo.OnFinish(rootOp, feature.OnFinish)
	return feature, nil
}

const (
	knuthFactor      uint64 = 11400714819323199488
	maxBodyParseSize        = 128 * 1024 // 128 KiB arbitrary value since it is not mentioned in the RFC

	// maxUint64 represented as a float64 because [math.MaxUint64] cannot be represented exactly as a float64
	// so we use the closest representable value that is MORE than 2^64-1 so it overflows
	// https://github.com/golang/go/issues/29463
	maxUint64 float64 = (1 << 64) - 1<<11
)

func (feature *DownwardRequestFeature) OnStart(op *httpsec.RoundTripOperation, args httpsec.RoundTripOperationArgs) {
	builder := addresses.NewAddressesBuilder().
		WithDownwardURL(args.URL).
		WithDownwardMethod(args.Method).
		WithDownwardRequestHeaders(args.Headers)

	// Increment the span metric for downward requests
	op.HandlerOp.ContextOperation.GetMetricsInstance().SumDownstreamRequestsCalls.Add(1)

	// Increment the internal sampling counter for downward requests
	requestCount := feature.downstreamRequestAnalysis.Add(1)

	hasDownstreamOverride := op.HandlerOp.HasDownstreamRequestOverride(op.URL())

	// Sampling algorithm based on:
	// https://docs.google.com/document/d/1DIGuCl1rkhx5swmGxKO7Je8Y4zvaobXBlgbm6C89yzU/edit?tab=t.0#heading=h.qawhep7pps5a
	if !hasDownstreamOverride && op.HandlerOp.DownstreamRequestBodyAnalysis() < feature.maxDownstreamRequestBodyAnalysis &&
		requestCount*knuthFactor <= uint64(feature.analysisSampleRate*maxUint64) {
		op.HandlerOp.IncrementDownstreamRequestBodyAnalysis()
		op.SetAnalyseBody()
	}

	if op.AnalyseBody() && args.Body != nil && *args.Body != nil && *args.Body != http.NoBody {
		encodable, err := body.NewEncodable(http.Header(args.Headers).Get("Content-Type"), args.Body, maxBodyParseSize)
		if err != nil {
			log.Debug("Unsupported response body content type or error reading body: %s", err.Error())
			telemetrylog.Warn("Unsupported request body content type or error reading body", slog.Any("error", telemetrylog.NewSafeError(err)))
		}
		op.SetRequestBody(encodable)
		builder = builder.WithDownwardRequestBody(encodable)
	}

	op.HandlerOp.Run(op, builder.Build())
}

func (feature *DownwardRequestFeature) OnFinish(op *httpsec.RoundTripOperation, args httpsec.RoundTripOperationRes) {
	builder := addresses.NewAddressesBuilder().
		WithDownwardResponseStatus(args.StatusCode).
		WithDownwardResponseHeaders(headersToLower(args.Headers))

	location := http.Header(args.Headers).Get("Location")
	isRedirect := args.StatusCode >= 300 && args.StatusCode <= 399 && location != ""

	var (
		analyzeBody         bool
		requestBody         = op.RequestBody()
		resubmitRequestBody = false
	)
	if override, found := op.HandlerOp.ConsumeDownstreamRequestOverride(op.URL()); found {
		// We are in a downstream request identified as part of a redirect chain. We use the original
		// sampling decision instead of making a new one.
		analyzeBody = override.AnalyzeBody
		requestBody = override.OriginalRequestBody
		// If we're at the end of a redirect chain, we re-submit the request body to assess data leakage
		// to un-trusted authorities.
		resubmitRequestBody = true
	} else {
		analyzeBody = op.AnalyseBody()
	}

	if isRedirect {
		opURL, err := url.Parse(op.URL())
		if err == nil {
			url, err := opURL.Parse(location)
			if err == nil {
				event := httpsec.DownstreamRequestOverride{
					DownstreamURL: url.String(),
					AnalyzeBody:   analyzeBody,
				}
				// Only HTTP 307 and 308 result in the body being re-submitted by the client.
				if args.StatusCode == http.StatusTemporaryRedirect || args.StatusCode == http.StatusPermanentRedirect {
					event.OriginalRequestBody = requestBody
				}
				dyngo.EmitData(op.HandlerOp, event)
			}
		}
	}

	if analyzeBody && !isRedirect && resubmitRequestBody && requestBody != nil {
		builder = builder.WithDownwardRequestBody(requestBody)
	}

	if analyzeBody && !isRedirect && args.Body != nil && *args.Body != nil && *args.Body != http.NoBody {
		encodable, err := body.NewEncodable(http.Header(args.Headers).Get("Content-Type"), args.Body, maxBodyParseSize)
		if err != nil {
			log.Debug("Unsupported response body content type or error reading body: %s", err.Error())
			telemetrylog.Warn("Unsupported response body content type or error reading body", slog.Any("error", telemetrylog.NewSafeError(err)))
		}
		builder = builder.WithDownwardResponseBody(encodable)
	}

	op.HandlerOp.Run(op, builder.Build())
}
