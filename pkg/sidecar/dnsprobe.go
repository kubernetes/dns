/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sidecar

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/datadog/datadog-go/statsd"
	"github.com/golang/glog"
	"github.com/miekg/dns"
)

// loopDelayer encapsulates the delay-loop timing logic. This
// externalizes it for testing.
type loopDelayer interface {
	// Start the delay loop, may sleep.
	Start(interval time.Duration)
	// Sleep the required amount of time taking into account the
	// `latency` of the loop body.
	Sleep(latency time.Duration)
}

type defaultLoopDelayer struct {
	interval time.Duration
}

func (d *defaultLoopDelayer) Start(interval time.Duration) {
	d.interval = interval
	// Stagger the start of the loop to avoid sending all probes at
	// exactly the same time.
	time.Sleep(time.Duration(rand.Int63n((int64)(d.interval))))
}

func (d *defaultLoopDelayer) Sleep(latency time.Duration) {
	sleepInterval := d.interval - latency
	if sleepInterval > 0 {
		glog.V(4).Infof("Sleeping %v", sleepInterval)
		time.Sleep(sleepInterval)
	}
}

type dnsProbe struct {
	DNSProbeOption

	lock               sync.Mutex
	lastResolveLatency time.Duration
	lastError          error
	statsdClient       *statsd.Client

	// loopDelay to use. If set to nil, dnsProbe will use
	// defaultLoopDelayer.
	delayer loopDelayer
}

func (p *dnsProbe) Start(options *Options) {
	glog.V(2).Infof("Starting dnsProbe %+v", p.DNSProbeOption)

	p.lastError = fmt.Errorf("waiting for first probe")

	http.HandleFunc("/healthcheck/"+p.Label, p.httpHandler)

	if p.delayer == nil {
		glog.V(4).Infof("Using defaultLoopDelayer")
		p.delayer = &defaultLoopDelayer{}
	}

	go p.loop()
}

func (p *dnsProbe) loop() {
	glog.V(4).Infof("Starting loop")
	p.delayer.Start(p.Interval)

	dnsClient := &dns.Client{}

	for {
		glog.V(4).Infof("Sending DNS request @%v %v", p.Server, p.Name)
		msg, latency, err := dnsClient.Exchange(p.msg(), p.Server)
		glog.V(4).Infof("Got response, err=%v after %v", err, latency)

		if err == nil && len(msg.Answer) == 0 {
			err = fmt.Errorf("no RRs for domain %q", p.Name)
		}

		p.update(err, latency)
		p.delayer.Sleep(latency)
	}
}

func (p *dnsProbe) update(err error, latency time.Duration) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if err == nil {
		p.lastResolveLatency = latency
		p.lastError = nil

		p.statsdClient.Histogram(fmt.Sprintf("%s.latency", p.Label), latency.Seconds()*1000, nil, 1)
	} else {
		glog.V(3).Infof("DNS resolution error for %v: %v", p.Label, err)
		p.lastResolveLatency = 0
		p.lastError = err

		p.statsdClient.Incr(fmt.Sprintf("%s.errors", p.Label), nil, 1)
	}
}

func (p *dnsProbe) msg() (msg *dns.Msg) {
	msg = new(dns.Msg)
	msg.Id = dns.Id()
	msg.RecursionDesired = true
	msg.Question = make([]dns.Question, 1)
	msg.Question[0] = dns.Question{
		Name:   p.Name,
		Qtype:  p.Type,
		Qclass: dns.ClassINET,
	}
	return
}

func (p *dnsProbe) httpHandler(w http.ResponseWriter, r *http.Request) {
	p.lock.Lock()
	defer p.lock.Unlock()

	response := struct {
		IsOk           bool
		LatencySeconds float64
		Err            string
	}{}

	if p.lastError == nil {
		response.IsOk = true
		response.LatencySeconds = p.lastResolveLatency.Seconds()

		if buf, err := json.Marshal(response); err == nil {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(buf)
		} else {
			glog.Errorf("JSON Marshal error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(([]byte)(fmt.Sprintf("Error: %v", err)))
		}
	} else {
		response.IsOk = false
		response.Err = p.lastError.Error()

		if buf, err := json.Marshal(response); err == nil {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write(buf)
		} else {
			glog.Errorf("JSON Marshal error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(([]byte)(fmt.Sprintf("Error: %v", err)))
		}
	}
}
