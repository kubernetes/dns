# Kubernetes DNS-Based Service Discovery

## 0 - About This Document

This document is a specification for DNS-based Kubernetes service
discovery. While service discovery in Kubernetes may be provided via
other protocols and mechanisms, DNS is very commonly used and is a
highly recommended add-on. The actual DNS service itself need not
be provided by the default Kube-DNS implementation. This document
is intended to provide a baseline for commonality between implementations.

## 1 - Schema Version

This document describes version 1.1.0 of the schema.

## 2 - Resource Records

Any DNS-based service discovery solution for Kubernetes must provide the
resource records (RRs) described below to be considered compliant with this
specification.

### 2.1 - Definitions

In the RR descriptions below, values not in angle brackets, `< >`,
are literals. The meaning of the values in angle brackets are defined
below or in the description of the specific record.

- `<zone>` = configured cluster domain, e.g. cluster.local
- `<ns>` = a Namespace
- `<ttl>` = the standard DNS time-to-live value for the record

In the RR descriptions below, the following definitions should be used for
words in _italics_.

_hostname_
  - In order of precedence, the _hostname_ of an endpoint is:
    - The value of the endpoint's `hostname` field.
    - A unique, system-assigned identifier for the endpoint. The exact format and source of this identifier is not prescribed by this specification. However, it must be possible to use this to identify a specific endpoint in the context of a Service. This is used in the event no explicit endpoint hostname is defined.

_ready_
  - An endpoint is considered _ready_ if its address is in the `addresses` field of the EndpointSubset object, or the corresponding service has the `service.alpha.kubernetes.io/tolerate-unready-endpoints` annotation set to `true`.

All comparisons between query data and data in Kubernetes are **case-insensitive**.

### 2.2 - Record for Schema Version

There must be a `TXT` record named `dns-version.<zone>.` that contains the
[semantic version](http://semver.org) of the DNS schema in use in this cluster.

- Record Format:
  - `dns-version.<zone>. <ttl> IN TXT <schema-version>`
- Question Example:
  - `dns-version.cluster.local. IN TXT`
- Answer Example:
  - `dns-version.cluster.local. 28800 IN TXT "1.1.0"`

### 2.3 - Records for a Service with ClusterIP

Given a Service named `<service>` in Namespace `<ns>` with ClusterIP
`<cluster-ip>`, the following records must exist.

#### 2.3.1 - `A`/`AAAA` Record

If the `<cluster-ip>` is an IPv4 address, an `A` record of the following
form must exist.

- Record Format:
  - `<service>.<ns>.svc.<zone>. <ttl> IN A <cluster-ip>`
- Question Example:
  - `kubernetes.default.svc.cluster.local. IN A`
- Answer Example:
  - `kubernetes.default.svc.cluster.local. 4 IN A 10.3.0.1`

If the `<cluster-ip>` is an IPv6 address, an `AAAA` record of the following
form must exist.

- Record Format:
  - `<service>.<ns>.svc.<zone>. <ttl> IN AAAA <cluster-ip>`
- Question Example:
  - `kubernetes.default.svc.cluster.local. IN AAAA`
- Answer Example:
  - `kubernetes.default.svc.cluster.local. 4 IN AAAA 2001:db8::1`

#### 2.3.2 - `SRV` Records

For each port in the Service with name `<port>` and number
`<port-number>` using protocol `<proto>`, an `SRV` record of the following
form must exist.

- Record Format:
   - `_<port>._<proto>.<service>.<ns>.svc.<zone>. <ttl> IN SRV <weight> <priority> <port-number> <service>.<ns>.svc.<zone>.`

The priority `<priority>` and weight `<weight>` are numbers as described
in [RFC2782](https://tools.ietf.org/html/rfc2782) and whose values are not
prescribed by this specification.

Unnamed ports do not have an `SRV` record.

- Question Example:
  - `_https._tcp.kubernetes.default.svc.cluster.local. IN SRV`
- Answer Example:
  - `_https._tcp.kubernetes.default.svc.cluster.local. 30 IN SRV 10 100 443 kubernetes.default.svc.cluster.local.`

The Additional section of the response may include the Service `A`/`AAAA` record
referred to in the `SRV` record.

#### 2.3.3 - `PTR` Record

Given an IPv4 Service ClusterIP `<a>.<b>.<c>.<d>`, a `PTR` record of the following
form must exist.

- Record Format:
  - `<d>.<c>.<b>.<a>.in-addr.arpa. <ttl> IN PTR <service>.<ns>.svc.<zone>.`
- Question Example:
  - `1.0.3.10.in-addr.arpa. IN PTR`
- Answer Example:
  - `1.0.3.10.in-addr.arpa. 14 IN PTR kubernetes.default.svc.cluster.local.`

Given an IPv6 Service ClusterIP represented in hexadecimal format without any simplification `<a1a2a3a4:b1b2b3b4:c1c2c3c4:d1d2d3d4:e1e2e3e4:f1f2f3f4:g1g2g3g4:h1h2h3h4>`, a `PTR` record as a sequence of nibbles in reverse order of the following form must exist.

- Record Format:
  - `h4.h3.h2.h1.g4.g3.g2.g1.f4.f3.f2.f1.e4.e3.e2.e1.d4.d3.d2.d1.c4.c3.c2.c1.b4.b3.b2.b1.a4.a3.a2.a1.ip6.arpa <ttl> IN PTR <service>.<ns>.svc.<zone>.`
- Question Example:
  - `1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa. IN PTR`
- Answer Example:
  - `1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa. 14 IN PTR kubernetes.default.svc.cluster.local.`

### 2.4 - Records for a Headless Service

Given a headless Service `<service>` in Namespace `<ns>` (i.e., a Service with
no ClusterIP), the following records must exist.

#### 2.4.1 - `A`/`AAAA` Records

There must be an `A` record for each _ready_ endpoint of the headless Service
with IPv4 address `<endpoint-ip>` as shown below. If there are no _ready_ endpoints
for the headless Service, the answer should be `NXDOMAIN`.

- Record Format:
  - `<service>.<ns>.svc.<zone>. <ttl> IN A <endpoint-ip>`
- Question Example:
  - `headless.default.svc.cluster.local. IN A`
- Answer Example:
```
    headless.default.svc.cluster.local. 4 IN A 10.3.0.1
    headless.default.svc.cluster.local. 4 IN A 10.3.0.2
    headless.default.svc.cluster.local. 4 IN A 10.3.0.3
```

There must also be an `A` record of the following form for each _ready_
endpoint with _hostname_ of `<hostname>` and IPv4 address `<endpoint-ip>`.
If there are multiple IPv4 addresses for a given _hostname_, then there
must be one such `A` record returned for each IP.

- Record Format:
  - `<hostname>.<service>.<ns>.svc.<zone>. <ttl> IN A <endpoint-ip>`
- Question Example:
  - `my-pet.headless.default.svc.cluster.local. IN A`
- Answer Example:
  - `my-pet.headless.default.svc.cluster.local. 4 IN A 10.3.0.100`

There must be an `AAAA` record for each _ready_ endpoint of the headless Service
with IPv6 address `<endpoint-ip>` as shown below. If there are no _ready_ endpoints
for the headless Service, the answer should be `NXDOMAIN`.

- Record Format:
  - `<service>.<ns>.svc.<zone>. <ttl> IN AAAA <endpoint-ip>`
- Question Example:
  - `headless.default.svc.cluster.local. IN AAAA`
- Answer Example:
```
    headless.default.svc.cluster.local. 4 IN AAAA 2001:db8::1
    headless.default.svc.cluster.local. 4 IN AAAA 2001:db8::2
    headless.default.svc.cluster.local. 4 IN AAAA 2001:db8::3
```

There must also be an `AAAA` record of the following form for each _ready_
endpoint with _hostname_ of `<hostname>` and IPv6 address `<endpoint-ip>`.
If there are multiple IPv6 addresses for a given _hostname_, then there
must be one such `AAAA` record returned for each IP.

- Record Format:
  - `<hostname>.<service>.<ns>.svc.<zone>. <ttl> IN AAAA <endpoint-ip>`
- Question Example:
  - `my-pet.headless.default.svc.cluster.local. IN AAAA`
- Answer Example:
  - `my-pet.headless.default.svc.cluster.local. 4 IN AAAA 2001:db8::1`

#### 2.4.2 - `SRV` Records

For each combination of _ready_ endpoint with _hostname_ of `<hostname>`, and
port in the Service with name `<port>` and number `<port-number>` using
protocol `<proto>`, an `SRV` record of the following form must exist.
- Record Format:
   - `_<port>._<proto>.<service>.<ns>.svc.<zone>. <ttl> IN SRV <weight> <priority> <port-number> <hostname>.<service>.<ns>.svc.<zone>.`

This implies that if there are **N** _ready_ endpoints and the Service
defines **M** named ports, there will be **N** :heavy_multiplication_x: **M**
`SRV` RRs for the Service.

The priority `<priority>` and weight `<weight>` are numbers as described
in [RFC2782](https://tools.ietf.org/html/rfc2782) and whose values are not
prescribed by this specification.

Unnamed ports do not have an `SRV` record.

- Question Example:
  - `_https._tcp.headless.default.svc.cluster.local. IN SRV`
- Answer Example:
```
        _https._tcp.headless.default.svc.cluster.local. 4 IN SRV 10 100 443 my-pet.headless.default.svc.cluster.local.
        _https._tcp.headless.default.svc.cluster.local. 4 IN SRV 10 100 443 my-pet-2.headless.default.svc.cluster.local.
        _https._tcp.headless.default.svc.cluster.local. 4 IN SRV 10 100 443 438934893.headless.default.svc.cluster.local.
```

The Additional section of the response may include the `A`/`AAAA` records
referred to in the `SRV` records.

#### 2.4.3 - `PTR` Records

Given a _ready_ endpoint with _hostname_ of `<hostname>` and IPv4 address
`<a>.<b>.<c>.<d>`, a `PTR` record of the following form must exist.
- Record Format:
  - `<d>.<c>.<b>.<a>.in-addr.arpa. <ttl> IN PTR <hostname>.<service>.<ns>.svc.<zone>.`
- Question Example:
  - `100.0.3.10.in-addr.arpa. IN PTR`
- Answer Example:
  - `100.0.3.10.in-addr.arpa. 14 IN PTR my-pet.headless.default.svc.cluster.local.`

Given a _ready_ endpoint with _hostname_ of `<hostname>` and IPv6 address
in hexadecimal format without any simplification `<a1a2a3a4:b1b2b3b4:c1c2c3c4:d1d2d3d4:e1e2e3e4:f1f2f3f4:g1g2g3g4:h1h2h3h4>`, a `PTR` record as a sequence of nibbles in reverse order of the following form must exist.

- Record Format:
  - `h4.h3.h2.h1.g4.g3.g2.g1.f4.f3.f2.f1.e4.e3.e2.e1.d4.d3.d2.d1.c4.c3.c2.c1.b4.b3.b2.b1.a4.a3.a2.a1.ip6.arpa <ttl> IN PTR <hostname>.<service>.<ns>.svc.<zone>.`
- Question Example:
  - `1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa. IN PTR`
- Answer Example:
  - `1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa. 14 IN PTR my-pet.headless.default.svc.cluster.local.`

### 2.5 - Records for External Name Services

Given a Service named `<service>` in Namespace `<ns>` with ExternalName `<extname>`,
a `CNAME` record named `<service>.<ns>.svc.<zone>` pointing to `<extname>` must
exist.

If the IP family of the service is IPv4:

- Record Format:
  - `<service>.<ns>.svc.<zone>. <ttl> IN CNAME <extname>.`
- Question Example:
  - `foo.default.svc.cluster.local. IN A`
- Answer Example:
  - `foo.default.svc.cluster.local. 10 IN CNAME www.example.com.`
  - `www.example.com. 28715 IN A 192.0.2.53`

If the IP family of the service is IPv6:

- Record Format:
  - `<service>.<ns>.svc.<zone>. <ttl> IN CNAME <extname>.`
- Question Example:
  - `foo.default.svc.cluster.local. IN AAAA`
- Answer Example:
  - `foo.default.svc.cluster.local. 10 IN CNAME www.example.com.`
  - `www.example.com. 28715 IN AAAA 2001:db8::1`


## 3 - Schema Extensions
Specific implementations may choose to extend this schema, but the RRs in this
document must be a subset of the RRs produced by the implementation.
