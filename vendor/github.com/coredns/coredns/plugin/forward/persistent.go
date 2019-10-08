package forward

import (
	"crypto/tls"
	"net"
	"sort"
	"time"

	"github.com/miekg/dns"
)

// a persistConn hold the dns.Conn and the last used time.
type persistConn struct {
	c    *dns.Conn
	used time.Time
}

// Transport hold the persistent cache.
type Transport struct {
	avgDialTime int64                     // kind of average time of dial time
	conns       map[string][]*persistConn // Buckets for udp, tcp and tcp-tls.
	expire      time.Duration             // After this duration a connection is expired.
	addr        string
	tlsConfig   *tls.Config

	dial  chan string
	yield chan *dns.Conn
	ret   chan *dns.Conn
	stop  chan bool
}

func newTransport(addr string) *Transport {
	t := &Transport{
		avgDialTime: int64(maxDialTimeout / 2),
		conns:       make(map[string][]*persistConn),
		expire:      defaultExpire,
		addr:        addr,
		dial:        make(chan string),
		yield:       make(chan *dns.Conn),
		ret:         make(chan *dns.Conn),
		stop:        make(chan bool),
	}
	return t
}

// len returns the number of connection, used for metrics. Can only be safely
// used inside connManager() because of data races.
func (t *Transport) len() int {
	l := 0
	for _, conns := range t.conns {
		l += len(conns)
	}
	return l
}

// connManagers manages the persistent connection cache for UDP and TCP.
func (t *Transport) connManager() {
	ticker := time.NewTicker(t.expire)
Wait:
	for {
		select {
		case proto := <-t.dial:
			// take the last used conn - complexity O(1)
			if stack := t.conns[proto]; len(stack) > 0 {
				pc := stack[len(stack)-1]
				if time.Since(pc.used) < t.expire {
					// Found one, remove from pool and return this conn.
					t.conns[proto] = stack[:len(stack)-1]
					t.ret <- pc.c
					continue Wait
				}
				// clear entire cache if the last conn is expired
				t.conns[proto] = nil
				// now, the connections being passed to closeConns() are not reachable from
				// transport methods anymore. So, it's safe to close them in a separate goroutine
				go closeConns(stack)
			}
			SocketGauge.WithLabelValues(t.addr).Set(float64(t.len()))

			t.ret <- nil

		case conn := <-t.yield:

			SocketGauge.WithLabelValues(t.addr).Set(float64(t.len() + 1))

			// no proto here, infer from config and conn
			if _, ok := conn.Conn.(*net.UDPConn); ok {
				t.conns["udp"] = append(t.conns["udp"], &persistConn{conn, time.Now()})
				continue Wait
			}

			if t.tlsConfig == nil {
				t.conns["tcp"] = append(t.conns["tcp"], &persistConn{conn, time.Now()})
				continue Wait
			}

			t.conns["tcp-tls"] = append(t.conns["tcp-tls"], &persistConn{conn, time.Now()})

		case <-ticker.C:
			t.cleanup(false)

		case <-t.stop:
			t.cleanup(true)
			close(t.ret)
			return
		}
	}
}

// closeConns closes connections.
func closeConns(conns []*persistConn) {
	for _, pc := range conns {
		pc.c.Close()
	}
}

// cleanup removes connections from cache.
func (t *Transport) cleanup(all bool) {
	staleTime := time.Now().Add(-t.expire)
	for proto, stack := range t.conns {
		if len(stack) == 0 {
			continue
		}
		if all {
			t.conns[proto] = nil
			// now, the connections being passed to closeConns() are not reachable from
			// transport methods anymore. So, it's safe to close them in a separate goroutine
			go closeConns(stack)
			continue
		}
		if stack[0].used.After(staleTime) {
			continue
		}

		// connections in stack are sorted by "used"
		good := sort.Search(len(stack), func(i int) bool {
			return stack[i].used.After(staleTime)
		})
		t.conns[proto] = stack[good:]
		// now, the connections being passed to closeConns() are not reachable from
		// transport methods anymore. So, it's safe to close them in a separate goroutine
		go closeConns(stack[:good])
	}
}

// Yield return the connection to transport for reuse.
func (t *Transport) Yield(c *dns.Conn) { t.yield <- c }

// Start starts the transport's connection manager.
func (t *Transport) Start() { go t.connManager() }

// Stop stops the transport's connection manager.
func (t *Transport) Stop() { close(t.stop) }

// SetExpire sets the connection expire time in transport.
func (t *Transport) SetExpire(expire time.Duration) { t.expire = expire }

// SetTLSConfig sets the TLS config in transport.
func (t *Transport) SetTLSConfig(cfg *tls.Config) { t.tlsConfig = cfg }

const (
	defaultExpire  = 10 * time.Second
	minDialTimeout = 1 * time.Second
	maxDialTimeout = 30 * time.Second

	// Some resolves might take quite a while, usually (cached) responses are fast. Set to 2s to give us some time to retry a different upstream.
	readTimeout = 2 * time.Second
)
