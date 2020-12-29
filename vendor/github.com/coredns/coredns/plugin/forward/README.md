# forward

## Name

*forward* - facilitates proxying DNS messages to upstream resolvers.

## Description

The *forward* plugin re-uses already opened sockets to the upstreams. It supports UDP, TCP and
DNS-over-TLS and uses in band health checking.

When it detects an error a health check is performed. This checks runs in a loop, performing each
check at a *0.5s* interval for as long as the upstream reports unhealthy. Once healthy we stop
health checking (until the next error). The health checks use a recursive DNS query (`. IN NS`)
to get upstream health. Any response that is not a network error (REFUSED, NOTIMPL, SERVFAIL, etc)
is taken as a healthy upstream. The health check uses the same protocol as specified in **TO**. If
`max_fails` is set to 0, no checking is performed and upstreams will always be considered healthy.

When *all* upstreams are down it assumes health checking as a mechanism has failed and will try to
connect to a random upstream (which may or may not work).

This plugin can only be used once per Server Block.

## Syntax

In its most basic form, a simple forwarder uses this syntax:

~~~
forward FROM TO...
~~~

* **FROM** is the base domain to match for the request to be forwarded.
* **TO...** are the destination endpoints to forward to. The **TO** syntax allows you to specify
  a protocol, `tls://9.9.9.9` or `dns://` (or no protocol) for plain DNS. The number of upstreams is
  limited to 15.

Multiple upstreams are randomized (see `policy`) on first use. When a healthy proxy returns an error
during the exchange the next upstream in the list is tried.

Extra knobs are available with an expanded syntax:

~~~
forward FROM TO... {
    except IGNORED_NAMES...
    force_tcp
    prefer_udp
    expire DURATION
    max_fails INTEGER
    tls CERT KEY CA
    tls_servername NAME
    policy random|round_robin|sequential
    health_check DURATION [no_rec]
    max_concurrent MAX
}
~~~

* **FROM** and **TO...** as above.
* **IGNORED_NAMES** in `except` is a space-separated list of domains to exclude from forwarding.
  Requests that match none of these names will be passed through.
* `force_tcp`, use TCP even when the request comes in over UDP.
* `prefer_udp`, try first using UDP even when the request comes in over TCP. If response is truncated
  (TC flag set in response) then do another attempt over TCP. In case if both `force_tcp` and
  `prefer_udp` options specified the `force_tcp` takes precedence.
* `max_fails` is the number of subsequent failed health checks that are needed before considering
  an upstream to be down. If 0, the upstream will never be marked as down (nor health checked).
  Default is 2.
* `expire` **DURATION**, expire (cached) connections after this time, the default is 10s.
* `tls` **CERT** **KEY** **CA** define the TLS properties for TLS connection. From 0 to 3 arguments can be
  provided with the meaning as described below

  * `tls` - no client authentication is used, and the system CAs are used to verify the server certificate
  * `tls` **CA** - no client authentication is used, and the file CA is used to verify the server certificate
  * `tls` **CERT** **KEY** - client authentication is used with the specified cert/key pair.
    The server certificate is verified with the system CAs
  * `tls` **CERT** **KEY**  **CA** - client authentication is used with the specified cert/key pair.
    The server certificate is verified using the specified CA file

* `tls_servername` **NAME** allows you to set a server name in the TLS configuration; for instance 9.9.9.9
  needs this to be set to `dns.quad9.net`. Multiple upstreams are still allowed in this scenario,
  but they have to use the same `tls_servername`. E.g. mixing 9.9.9.9 (QuadDNS) with 1.1.1.1
  (Cloudflare) will not work.
* `policy` specifies the policy to use for selecting upstream servers. The default is `random`.
  * `random` is a policy that implements random upstream selection.
  * `round_robin` is a policy that selects hosts based on round robin ordering.
  * `sequential` is a policy that selects hosts based on sequential ordering.
* `health_check` configure the behaviour of health checking of the upstream servers
  * `<duration>` - use a different duration for health checking, the default duration is 0.5s.
  * `no_rec` - optional argument that sets the RecursionDesired-flag of the dns-query used in health checking to `false`.
    The flag is default `true`.
* `max_concurrent` **MAX** will limit the number of concurrent queries to **MAX**.  Any new query that would
  raise the number of concurrent queries above the **MAX** will result in a SERVFAIL response. This
  response does not count as a health failure. When choosing a value for **MAX**, pick a number
  at least greater than the expected *upstream query rate* * *latency* of the upstream servers.
  As an upper bound for **MAX**, consider that each concurrent query will use about 2kb of memory.

Also note the TLS config is "global" for the whole forwarding proxy if you need a different
`tls-name` for different upstreams you're out of luck.

On each endpoint, the timeouts for communication are set as follows:

* The dial timeout by default is 30s, and can decrease automatically down to 100ms based on early results.
* The read timeout is static at 2s.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

* `coredns_forward_requests_total{to}` - query count per upstream.
* `coredns_forward_responses_total{to}` - Counter of responses received per upstream.
* `coredns_forward_request_duration_seconds{to}` - duration per upstream interaction.
* `coredns_forward_responses_total{to, rcode}` - count of RCODEs per upstream.
* `coredns_forward_healthcheck_failures_total{to}` - number of failed health checks per upstream.
* `coredns_forward_healthcheck_broken_total{}` - counter of when all upstreams are unhealthy,
  and we are randomly (this always uses the `random` policy) spraying to an upstream.
* `coredns_forward_max_concurrent_rejects_total{}` - counter of the number of queries rejected because the
  number of concurrent queries were at maximum.
* `coredns_forward_conn_cache_hits_total{to, proto}` - counter of connection cache hits per upstream and protocol.
* `coredns_forward_conn_cache_misses_total{to, proto}` - counter of connection cache misses per upstream and protocol.
Where `to` is one of the upstream servers (**TO** from the config), `rcode` is the returned RCODE
from the upstream, `proto` is the transport protocol like `udp`, `tcp`, `tcp-tls`.

## Examples

Proxy all requests within `example.org.` to a nameserver running on a different port:

~~~ corefile
example.org {
    forward . 127.0.0.1:9005
}
~~~

Load balance all requests between three resolvers, one of which has a IPv6 address.

~~~ corefile
. {
    forward . 10.0.0.10:53 10.0.0.11:1053 [2003::1]:53
}
~~~

Forward everything except requests to `example.org`

~~~ corefile
. {
    forward . 10.0.0.10:1234 {
        except example.org
    }
}
~~~

Proxy everything except `example.org` using the host's `resolv.conf`'s nameservers:

~~~ corefile
. {
    forward . /etc/resolv.conf {
        except example.org
    }
}
~~~

Proxy all requests to 9.9.9.9 using the DNS-over-TLS protocol, and cache every answer for up to 30
seconds. Note the `tls_servername` is mandatory if you want a working setup, as 9.9.9.9 can't be
used in the TLS negotiation. Also set the health check duration to 5s to not completely swamp the
service with health checks.

~~~ corefile
. {
    forward . tls://9.9.9.9 {
       tls_servername dns.quad9.net
       health_check 5s
    }
    cache 30
}
~~~

Or with multiple upstreams from the same provider

~~~ corefile
. {
    forward . tls://1.1.1.1 tls://1.0.0.1 {
       tls_servername cloudflare-dns.com
       health_check 5s
    }
    cache 30
}
~~~

## Bugs

The TLS config is global for the whole forwarding proxy if you need a different `tls_servername` for
different upstreams you're out of luck.

## Also See

[RFC 7858](https://tools.ietf.org/html/rfc7858) for DNS over TLS.
