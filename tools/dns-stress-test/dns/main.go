package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	math_rand "math/rand"
	"net"
	"os"
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"github.com/golang/glog"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
)

var names []string

func main() {
	math_rand.Seed(time.Now().UnixNano())

	var namesPath string
	flag.StringVar(&namesPath, "f", namesPath, "path to file containing list of DNS names to resolve")

	flag.Set("logtostderr", "true")
	flag.Parse()

	namesString := `
google.com
yahoo.com
facebook.com
ebay.com
amazon.com
microsoft.com
`

	if namesPath != "" {
		b, err := ioutil.ReadFile(namesPath)
		if err != nil {
			glog.Fatalf("error reading %s: %v", namesPath, err)
		}

		namesString = string(b)
	}

	for _, s := range strings.Split(namesString, "\n") {
		s = strings.TrimSpace(s)
		if s != "" {
			names = append(names, s)
		}
	}

	fmt.Printf("dns stress test\n")

	ctx := context.Background()

	resource := &monitoredrespb.MonitoredResource{
		Type: "k8s_container",
		Labels: map[string]string{
			"namespace_id":   os.Getenv("MY_POD_NAMESPACE"),
			"pod_id":         os.Getenv("MY_POD_NAME"),
			"container_name": os.Getenv("MY_CONTAINER_NAME"),
		},
	}

	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: os.Getenv("GCP_PROJECT"),
		Resource:  resource,
	})
	if err != nil {
		glog.Fatalf("unexpected error initializing stackdriver: %v", err)
	}

	// Export to Stackdriver Monitoring.
	view.RegisterExporter(exporter)

	// Export to Stackdriver Trace.
	timeSampler := NewSamplingExporter(exporter, time.Millisecond*50, 0.01)
	trace.RegisterExporter(timeSampler)

	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	err = run(ctx)

	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func pickHostname() string {
	return names[math_rand.Intn(len(names))]
}

func run(ctx context.Context) error {
	for {
		host := pickHostname()
		err := resolve(ctx, host)
		if err != nil {
			glog.Infof("error resolving host %q: %v", host, err)
			continue
		}

		// At most 100 qps (per pod)
		time.Sleep(10 * time.Millisecond)
	}
}

func resolve(ctx context.Context, host string) error {
	ctx, span := trace.StartSpan(ctx, "dns lookup")
	span.AddAttributes(trace.StringAttribute("host", host), trace.StringAttribute("node", os.Getenv("MY_NODE_NAME")))
	span.AddAttributes(trace.StringAttribute("variant", os.Getenv("MY_VARIANT")))
	defer span.End()

	span.Annotate([]trace.Attribute{trace.StringAttribute("host", host)}, "doing dns lookup")

	ips, err := net.LookupIP(host)
	if err != nil {
		glog.Infof("error looking up host %q: %v", host, err)

		span.Annotate([]trace.Attribute{trace.StringAttribute("error", err.Error())}, "error from lookup")
		span.SetStatus(trace.Status{Code: trace.StatusCodeUnknown, Message: err.Error()})

		// TODO: Expose information from span ?
		return fmt.Errorf("error from lookup on %s: %v", host, err)
	}

	if len(ips) == 0 {
		span.Annotate(nil, "no ips from lookup")
		span.SetStatus(trace.Status{Code: trace.StatusCodeUnknown})
		return fmt.Errorf("no ips from lookup of %s", host)
	}

	return nil
}
