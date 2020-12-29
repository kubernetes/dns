// Package trace implements OpenTracing-based tracing
package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	_ "github.com/coredns/coredns/plugin/pkg/trace" // Plugin the trace package.
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	zipkinot "github.com/openzipkin-contrib/zipkin-go-opentracing"
	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	tagName                 = "coredns.io/name"
	tagType                 = "coredns.io/type"
	tagRcode                = "coredns.io/rcode"
	tagProto                = "coredns.io/proto"
	tagRemote               = "coredns.io/remote"
	defaultTopLevelSpanName = "servedns"
)

type trace struct {
	count uint64 // as per Go spec, needs to be first element in a struct

	Next                 plugin.Handler
	Endpoint             string
	EndpointType         string
	tracer               ot.Tracer
	serviceEndpoint      string
	serviceName          string
	clientServer         bool
	every                uint64
	datadogAnalyticsRate float64
	Once                 sync.Once
}

func (t *trace) Tracer() ot.Tracer {
	return t.tracer
}

// OnStartup sets up the tracer
func (t *trace) OnStartup() error {
	var err error
	t.Once.Do(func() {
		switch t.EndpointType {
		case "zipkin":
			err = t.setupZipkin()
		case "datadog":
			tracer := opentracer.New(
				tracer.WithAgentAddr(t.Endpoint),
				tracer.WithDebugMode(log.D.Value()),
				tracer.WithGlobalTag(ext.SpanTypeDNS, true),
				tracer.WithServiceName(t.serviceName),
				tracer.WithAnalyticsRate(t.datadogAnalyticsRate),
			)
			t.tracer = tracer
		default:
			err = fmt.Errorf("unknown endpoint type: %s", t.EndpointType)
		}
	})
	return err
}

func (t *trace) setupZipkin() error {
	reporter := zipkinhttp.NewReporter(t.Endpoint)
	recorder, err := zipkin.NewEndpoint(t.serviceName, t.serviceEndpoint)
	if err != nil {
		log.Warningf("build Zipkin endpoint found err: %v", err)
	}
	tracer, err := zipkin.NewTracer(
		reporter,
		zipkin.WithLocalEndpoint(recorder),
	)
	if err != nil {
		return err
	}
	t.tracer = zipkinot.Wrap(tracer)
	return err
}

// Name implements the Handler interface.
func (t *trace) Name() string { return "trace" }

// ServeDNS implements the plugin.Handle interface.
func (t *trace) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	trace := false
	if t.every > 0 {
		queryNr := atomic.AddUint64(&t.count, 1)

		if queryNr%t.every == 0 {
			trace = true
		}
	}
	span := ot.SpanFromContext(ctx)
	if !trace || span != nil {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	req := request.Request{W: w, Req: r}
	span = t.Tracer().StartSpan(defaultTopLevelSpanName)
	defer span.Finish()

	rw := dnstest.NewRecorder(w)
	ctx = ot.ContextWithSpan(ctx, span)
	status, err := plugin.NextOrFailure(t.Name(), t.Next, ctx, rw, r)

	span.SetTag(tagName, req.Name())
	span.SetTag(tagType, req.Type())
	span.SetTag(tagProto, req.Proto())
	span.SetTag(tagRemote, req.IP())
	span.SetTag(tagRcode, rcode.ToString(rw.Rcode))

	return status, err
}
