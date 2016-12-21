# Kubernetes DNS

[![Build Status](https://travis-ci.org/kubernetes/dns.svg?branch=master)](https://travis-ci.org/kubernetes/dns)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes/dns/badge.svg?branch=master)](https://coveralls.io/github/kubernetes/dns?branch=master)

This is the repository for [Kubernetes DNS](http://kubernetes.io/docs/admin/dns/).

## Subprojects

* [sidecar](docs/sidecar/README.md)

## Building

`make` targets:

| target | description |
| ---- | ---- |
|all, build | build all binaries |
|containers | build the containers |
|push       | push containers to the registry |
|help       | this help message |
|version    | show package version |
|{build,containers,push}-ARCH | do action for specific ARCH |
|all-{build,containers,push}  | do action for all ARCH |
|only-push-BINARY             | push just BINARY |

* Setting `VERBOSE=1` will show additional build logging.
* Setting `VERSION` will override the container version tag.
