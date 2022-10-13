package forward

import (
	"crypto/tls"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/miekg/dns"
)

// HealthChecker checks the upstream health.
type HealthChecker interface {
	Check(*Proxy) error
	SetTLSConfig(*tls.Config)
	SetRecursionDesired(bool)
	GetRecursionDesired() bool
}

// dnsHc is a health checker for a DNS endpoint (DNS, and DoT).
type dnsHc struct {
	c                *dns.Client
	recursionDesired bool
}

var (
	hcReadTimeout  = 1 * time.Second
	hcWriteTimeout = 1 * time.Second
)

// NewHealthChecker returns a new HealthChecker based on transport.
func NewHealthChecker(trans string, recursionDesired bool) HealthChecker {
	switch trans {
	case transport.DNS, transport.TLS:
		c := new(dns.Client)
		c.Net = "udp"
		c.ReadTimeout = hcReadTimeout
		c.WriteTimeout = hcWriteTimeout

		return &dnsHc{c: c, recursionDesired: recursionDesired}
	}

	log.Warningf("No healthchecker for transport %q", trans)
	return nil
}

func (h *dnsHc) SetTLSConfig(cfg *tls.Config) {
	h.c.Net = "tcp-tls"
	h.c.TLSConfig = cfg
}

func (h *dnsHc) SetRecursionDesired(recursionDesired bool) {
	h.recursionDesired = recursionDesired
}
func (h *dnsHc) GetRecursionDesired() bool {
	return h.recursionDesired
}

// For HC we send to . IN NS +[no]rec message to the upstream. Dial timeouts and empty
// replies are considered fails, basically anything else constitutes a healthy upstream.

// Check is used as the up.Func in the up.Probe.
func (h *dnsHc) Check(p *Proxy) error {
	err := h.send(p.addr)
	if err != nil {
		HealthcheckFailureCount.WithLabelValues(p.addr).Add(1)
		atomic.AddUint32(&p.fails, 1)
		return err
	}

	atomic.StoreUint32(&p.fails, 0)
	return nil
}

func (h *dnsHc) send(addr string) error {
	ping := new(dns.Msg)
	ping.SetQuestion(".", dns.TypeNS)
	ping.MsgHdr.RecursionDesired = h.recursionDesired

	m, _, err := h.c.Exchange(ping, addr)
	// If we got a header, we're alright, basically only care about I/O errors 'n stuff.
	if err != nil && m != nil {
		// Silly check, something sane came back.
		if m.Response || m.Opcode == dns.OpcodeQuery {
			err = nil
		}
	}

	return err
}
