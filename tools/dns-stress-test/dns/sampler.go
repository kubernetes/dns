package main

import (
	"math/rand"
	"time"

	"go.opencensus.io/trace"
)

type samplingExporter struct {
	inner       trace.Exporter
	threshold   time.Duration
	probability float32
}

func NewSamplingExporter(inner trace.Exporter, threshold time.Duration, probability float32) trace.Exporter {
	return &samplingExporter{
		inner:       inner,
		threshold:   threshold,
		probability: probability,
	}
}

func (x *samplingExporter) ExportSpan(s *trace.SpanData) {
	duration := s.EndTime.Sub(s.StartTime)

	export := false
	if duration >= x.threshold {
		export = true
	} else {
		r := rand.Float32()
		export = r <= x.probability
	}

	if export {
		x.inner.ExportSpan(s)
	}
}
