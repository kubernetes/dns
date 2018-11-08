## Overview

This example allows for use of the [node-local-dns
cache](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/dns/nodelocaldns),
without requiring the kubelet `--cluster-dns` flag to be altered.  In
particular, this means it can be used on GKE clusters.

This is experimental, but feedback is welcome - please tag @justinsb.

## Warning: Network policy

If running with network policy, please see the [README for
node-local-dns](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/dns/nodelocaldns#network-policy-and-dns-connectivity);
network policy will likely need to be configured for the node-local-dns agent.

## Other caveats

* If the node-local-dns agent crashes, DNS resolution will not function on that
  node until it is restarted.

* This configuration must be removed before installing the node-local-dns cache.

## How it works

Normally the node-local-dns agent:
* uses an unused IP (typically `169.254.20.10`)
* the kubelet `--cluster-dns` flag is used to specify that pods should use that
  IP address (`169.254.20.10`) as their DNS server
* CoreDNS runs as a daemonset on every node, configured to listen to the
  internal IP (`169.254.20.10`)
* CoreDNS caches results locally, but forwards uncached queries upstream to the
  "real" kube-dns service
* The node-local-dns agent configures IP tables rules to avoid conntrack / NAT

In this mode, we instead intercept the existing kube-dns service IP a few
things:
* We configure the node-local-dns agent to intercept the kube-dns service IP
* kubelet is already configured to send queries to that service, by default
* We configure a second service to act as the "real" kube-dns service
* When the node-local-dns agent configures the kube-dns service IP to avoid
  conntrack/NAT, this takes precedence over the normal DNS service routing.

## Installation

A script is provided, simply run `./install.sh`

## Removal

Removal is more complicated that installation.  We can remove the daemonset, and as
part of pod shutdown the node-local-dns cache should remove the IP interception
rules.  However, if something goes wrong with the removal, the IP interception rules
will remain in place, but the node-local-dns cache will not be running to serve
the intercepted traffic, and DNS lookup will be broken on that node.  However,
restarting the machine will remove the IP interception rules, so if this is done
as part of a cluster update the system will self-heal.

The procedure therefore is:

* Run `./uninstall.sh`
* Upgrade cluster
* Remove uncached service using `kubectl delete service -n kube-system
  kube-dns-uncached` (not critical, just for cleanup)

## IP interception rules

If you would like to verify that the IP interception rules are present or
removed, the `list-iptables-rules.sh` will list the interception rules.

For example, the output might look like:

```
---------------------------------------------------------------------------
kube-proxy-gke-jsb-test-default-pool-751c2b6d-pnls
---------------------------------------------------------------------------
-P PREROUTING ACCEPT
-P OUTPUT ACCEPT
-A PREROUTING -d 10.23.240.10/32 -p udp -m udp --dport 53 -j NOTRACK
-A PREROUTING -d 10.23.240.10/32 -p tcp -m tcp --dport 53 -j NOTRACK
-A OUTPUT -s 10.23.240.10/32 -p udp -m udp --sport 53 -j NOTRACK
-A OUTPUT -s 10.23.240.10/32 -p tcp -m tcp --sport 53 -j NOTRACK

...

```

These rules are intercepting the ClusterIP for kube-dns (`10.23.240.10`), opting
them out of conntracking and therefore of NAT.

Finally, a script is included that can remove the interception iptables rules
directly.  It can be run if `list-iptables-rules.sh` is showing any rules
remaining after the daemonset has been removed.  It is
`remove-iptables-rules.sh`
