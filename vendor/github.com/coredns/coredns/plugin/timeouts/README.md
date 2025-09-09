# timeouts

## Name

*timeouts* - allows you to configure the server read, write and idle timeouts for the TCP, TLS, DoH and DoQ (idle only) servers.

## Description

CoreDNS is configured with sensible timeouts for server connections by default.
However in some cases for example where CoreDNS is serving over a slow mobile
data connection the default timeouts are not optimal.

Additionally some routers hold open connections when using DNS over TLS or DNS
over HTTPS. Allowing a longer idle timeout helps performance and reduces issues
with such routers.

The *timeouts* "plugin" allows you to configure CoreDNS server read, write and
idle timeouts.

## Syntax

~~~ txt
timeouts {
	read DURATION
	write DURATION
	idle DURATION
}
~~~

For any timeouts that are not provided, default values are used which may vary
depending on the server type. At least one timeout must be specified otherwise
the entire timeouts block should be omitted.

## Examples

Start a DNS-over-TLS server that picks up incoming DNS-over-TLS queries on port
5553 and uses the nameservers defined in `/etc/resolv.conf` to resolve the
query. This proxy path uses plain old DNS. A 10 second read timeout, 20
second write timeout and a 60 second idle timeout have been configured.

~~~
tls://.:5553 {
	tls cert.pem key.pem ca.pem
	timeouts {
		read 10s
		write 20s
		idle 60s
	}
	forward . /etc/resolv.conf
}
~~~

Start a DNS-over-HTTPS server that is similar to the previous example. Only the
read timeout has been configured for 1 minute.

~~~
https://. {
	tls cert.pem key.pem ca.pem
	timeouts {
		read 1m
	}
	forward . /etc/resolv.conf
}
~~~

Start a DNS-over-QUIC server that has the idle timeout set to two minutes.

~~~
quic://.:853 {
	tls cert.pem key.pem ca.pem
	timeouts {
		idle 2m
	}
	forward . /etc/resolv.conf
}
~~~

Start a standard TCP/UDP server on port 1053. A read and write timeout has been
configured. The timeouts are only applied to the TCP side of the server.

~~~
.:1053 {
	timeouts {
		read 15s
		write 30s
	}
	forward . /etc/resolv.conf
}
~~~
