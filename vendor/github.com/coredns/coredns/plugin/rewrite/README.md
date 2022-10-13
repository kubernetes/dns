# rewrite

## Name

*rewrite* - performs internal message rewriting.

## Description

Rewrites are invisible to the client. There are simple rewrites (fast) and complex rewrites
(slower), but they're powerful enough to accommodate most dynamic back-end applications.

## Syntax

A simplified/easy-to-digest syntax for *rewrite* is...
~~~
rewrite [continue|stop] FIELD [TYPE] [(FROM TO)|TTL] [OPTIONS]
~~~

* **FIELD** indicates what part of the request/response is being re-written.

   * `type` - the type field of the request will be rewritten. FROM/TO must be a DNS record type (`A`, `MX`, etc.);
e.g., to rewrite ANY queries to HINFO, use `rewrite type ANY HINFO`.
   * `name` - the query name in the _request_ is rewritten; by default this is a full match of the
     name, e.g., `rewrite name example.net example.org`. Other match types are supported, see the **Name Field Rewrites** section below.
   * `class` - the class of the message will be rewritten. FROM/TO must be a DNS class type (`IN`, `CH`, or `HS`); e.g., to rewrite CH queries to IN use `rewrite class CH IN`.
   * `edns0` - an EDNS0 option can be appended to the request as described below in the **EDNS0 Options** section.
   * `ttl` - the TTL value in the _response_ is rewritten.

* **TYPE** this optional element can be specified for a `name` or `ttl` field.
  If not given type `exact` will be assumed. If options should be specified the
  type must be given.
* **FROM** is the name (exact, suffix, prefix, substring, or regex) or type to match
* **TO** is the destination name or type to rewrite to
* **TTL** is the number of seconds to set the TTL value to (only for field `ttl`)

* **OPTIONS**

  for field `name` further options are possible controlling the response rewrites.
  All name matching types support the following options

     * `answer auto` - the names in the _response_ is rewritten in a best effort manner.
     * `answer name FROM TO` - the query name in the _response_ is rewritten matching the from regex pattern.
     * `answer value FROM TO` - the names in the _response_ is rewritten matching the from regex pattern.

  See below in the **Response Rewrites** section for further details.

If you specify multiple rules and an incoming query matches multiple rules, the rewrite
will behave as follows:

   * `continue` will continue applying the next rule in the rule list.
   * `stop` will consider the current rule the last rule and will not continue.  The default behaviour is `stop`

## Examples

### Name Field Rewrites

The `rewrite` plugin offers the ability to match the name in the question section of
a DNS request. The match could be exact, a substring match, or based on a prefix, suffix, or regular
expression. If the newly used name is not a legal domain name, the plugin returns an error to the
client.

The syntax for name rewriting is as follows:

```
rewrite [continue|stop] name [exact|prefix|suffix|substring|regex] STRING STRING [OPTIONS]
```

The match type, e.g., `exact`, `substring`, etc., triggers rewrite:

* **exact** (default): on an exact match of the name in the question section of a request
* **substring**: on a partial match of the name in the question section of a request
* **prefix**: when the name begins with the matching string
* **suffix**: when the name ends with the matching string
* **regex**: when the name in the question section of a request matches a regular expression

If the match type is omitted, the `exact` match type is assumed. If OPTIONS are
given, the type must be specified.

The following instruction allows rewriting names in the query that
contain the substring `service.us-west-1.example.org`:

```
rewrite name substring service.us-west-1.example.org service.us-west-1.consul
```

Thus:

* Incoming Request Name: `ftp.service.us-west-1.example.org`
* Rewritten Request Name: `ftp.service.us-west-1.consul`

The following instruction uses regular expressions. Names in requests
matching the regular expression `(.*)-(us-west-1)\.example\.org` are replaced with
`{1}.service.{2}.consul`, where `{1}` and `{2}` are regular expression match groups.

```
rewrite name regex (.*)-(us-west-1)\.example\.org {1}.service.{2}.consul
```

Thus:

* Incoming Request Name: `ftp-us-west-1.example.org`
* Rewritten Request Name: `ftp.service.us-west-1.consul`

The following example rewrites the `schmoogle.com` suffix to `google.com`.

~~~
rewrite name suffix .schmoogle.com. .google.com.
~~~

### Response Rewrites

When rewriting incoming DNS requests' names (field `name`), CoreDNS re-writes
the `QUESTION SECTION`
section of the requests. It may be necessary to rewrite the `ANSWER SECTION` of the
requests, because some DNS resolvers treat mismatches between the `QUESTION SECTION`
and `ANSWER SECTION` as a man-in-the-middle attack (MITM).

For example, a user tries to resolve `ftp-us-west-1.coredns.rocks`. The
CoreDNS configuration file has the following rule:

```
rewrite name regex (.*)-(us-west-1)\.coredns\.rocks {1}.service.{2}.consul
```

CoreDNS rewrote the request from `ftp-us-west-1.coredns.rocks` to
`ftp.service.us-west-1.consul` and ultimately resolved it to 3 records.
The resolved records, in the `ANSWER SECTION` below, were not from `coredns.rocks`, but
rather from `service.us-west-1.consul`.


```
$ dig @10.1.1.1 ftp-us-west-1.coredns.rocks

;; QUESTION SECTION:
;ftp-us-west-1.coredns.rocks. IN A

;; ANSWER SECTION:
ftp.service.us-west-1.consul. 0    IN A    10.10.10.10
ftp.service.us-west-1.consul. 0    IN A    10.20.20.20
ftp.service.us-west-1.consul. 0    IN A    10.30.30.30
```

The above is a mismatch between the question asked and the answer provided.

There are three possibilities to specify an answer rewrite:
- A rewrite can request a best effort answer rewrite by adding the option `answer auto`.
- A rewrite may specify a dedicated regex based response name rewrite with the
  `answer name FROM TO` option.
- A regex based rewrite of record values like `CNAME`, `SRV`, etc, can be requested by
  an `answer value FROM TO` option.

Hereby FROM/TO follow the rules for the `regex` name rewrite syntax.

#### Auto Response Name Rewrite

The following configuration snippet allows for rewriting of the
`ANSWER SECTION` according to the rewrite of the `QUESTION SECTION`:

```
    rewrite stop {
        name suffix .coredns.rocks .service.consul answer auto
    }
```

Any occurrence of the rewritten question in the answer is mapped
back to the original value before the rewrite.

Please note that answers for rewrites of type `exact` are always rewritten.
For a `suffix` name rule `auto` leads to a reverse suffix response rewrite,
exchanging FROM and TO from the rewrite request.

#### Explicit Response Name Rewrite

The following configuration snippet allows for rewriting of the
`ANSWER SECTION`, provided that the `QUESTION SECTION` was rewritten:

```
    rewrite stop {
        name regex (.*)-(us-west-1)\.coredns\.rocks {1}.service.{2}.consul
        answer name (.*)\.service\.(us-west-1)\.consul {1}-{2}.coredns.rocks
    }
```

Now, the `ANSWER SECTION` matches the `QUESTION SECTION`:

```
$ dig @10.1.1.1 ftp-us-west-1.coredns.rocks

;; QUESTION SECTION:
;ftp-us-west-1.coredns.rocks. IN A

;; ANSWER SECTION:
ftp-us-west-1.coredns.rocks. 0    IN A    10.10.10.10
ftp-us-west-1.coredns.rocks. 0    IN A    10.20.20.20
ftp-us-west-1.coredns.rocks. 0    IN A    10.30.30.30
```

#### Rewriting other Response Values

It is also possible to rewrite other values returned in the DNS response records
(e.g. the server names returned in `SRV` and `MX` records). This can be enabled by adding
the `answer value FROM TO` option to a name rule as specified below. `answer value` takes a
regular expression and a rewrite name as parameters and works in the same way as the
`answer name` rule.

Note that names in the `AUTHORITY SECTION` and `ADDITIONAL SECTION` will also be
rewritten following the specified rules. The names returned by the following
record types: `CNAME`, `DNAME`, `SOA`, `SRV`, `MX`, `NAPTR`, `NS` will be rewritten
if the `answer value` rule is specified.

The syntax for the rewrite of DNS request and response is as follows:

```
rewrite [continue|stop] {
    name regex STRING STRING
    answer name STRING STRING
    [answer value STRING STRING]
}
```

Note that the above syntax is strict.  For response rewrites, only `name`
rules are allowed to match the question section. The answer rewrite must be
after the name, as in the syntax example.

#### Multiple Response Rewrites

`name` and `value` rewrites can be chained by appending multiple answer rewrite
options. For all occurrences but the first one the keyword `answer` might be
omitted.

```options
answer (auto | (name|value FROM TO)) { [answer] (auto | (name|value FROM TO)) }
```

For example:
```
rewrite [continue|stop] name regex FROM TO answer name FROM TO [answer] value FROM TO
```

When using `exact` name rewrite rules, the answer gets rewritten automatically,
and there is no need to define `answer name auto`. But it is still possible to define
additional `answer value` and `answer value` options.

The rule below rewrites the name in a request from `RED` to `BLUE`, and subsequently
rewrites the name in a corresponding response from `BLUE` to `RED`. The
client in the request would see only `RED` and no `BLUE`.

```
rewrite [continue|stop] name exact RED BLUE
```

### TTL Field Rewrites

At times, the need to rewrite a TTL value could arise. For example, a DNS server
may not cache records with a TTL of zero (`0`). An administrator
may want to increase the TTL to ensure it is cached, e.g., by increasing it to 15 seconds.

In the below example, the TTL in the answers for `coredns.rocks` domain are
being set to `15`:

```
    rewrite continue {
        ttl regex (.*)\.coredns\.rocks 15
    }
```

By the same token, an administrator may use this feature to prevent or limit caching by
setting the TTL value really low.


The syntax for the TTL rewrite rule is as follows. The meaning of
`exact|prefix|suffix|substring|regex` is the same as with the name rewrite rules.
An omitted type is defaulted to `exact`.

```
rewrite [continue|stop] ttl [exact|prefix|suffix|substring|regex] STRING SECONDS
```

## EDNS0 Options

Using the FIELD edns0, you can set, append, or replace specific EDNS0 options in the request.

* `replace` will modify any "matching" option with the specified option. The criteria for "matching" varies based on EDNS0 type.
* `append` will add the option only if no matching option exists
* `set` will modify a matching option or add one if none is found

Currently supported are `EDNS0_LOCAL`, `EDNS0_NSID` and `EDNS0_SUBNET`.

### EDNS0_LOCAL

This has two fields, code and data. A match is defined as having the same code. Data may be a string or a variable.

* A string data is treated as hex if it starts with `0x`. Example:

~~~ corefile
. {
    rewrite edns0 local set 0xffee 0x61626364
    whoami
}
~~~

rewrites the first local option with code 0xffee, setting the data to "abcd". This is equivalent to:

~~~ corefile
. {
    rewrite edns0 local set 0xffee abcd
}
~~~

* A variable data is specified with a pair of curly brackets `{}`. Following are the supported variables:
  {qname}, {qtype}, {client_ip}, {client_port}, {protocol}, {server_ip}, {server_port}.

* If the metadata plugin is enabled, then labels are supported as variables if they are presented within curly brackets.
The variable data will be replaced with the value associated with that label. If that label is not provided,
the variable will be silently substituted with an empty string.

Examples:

~~~
rewrite edns0 local set 0xffee {client_ip}
~~~

The following example uses metadata and an imaginary "some-plugin" that would provide "some-label" as metadata information.

~~~
metadata
some-plugin
rewrite edns0 local set 0xffee {some-plugin/some-label}
~~~

### EDNS0_NSID

This has no fields; it will add an NSID option with an empty string for the NSID. If the option already exists
and the action is `replace` or `set`, then the NSID in the option will be set to the empty string.

### EDNS0_SUBNET

This has two fields,  IPv4 bitmask length and IPv6 bitmask length. The bitmask
length is used to extract the client subnet from the source IP address in the query.

Example:

~~~
rewrite edns0 subnet set 24 56
~~~

* If the query's source IP address is an IPv4 address, the first 24 bits in the IP will be the network subnet.
* If the query's source IP address is an IPv6 address, the first 56 bits in the IP will be the network subnet.
