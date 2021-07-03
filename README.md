# Kubernetes DNS

[![Build Status](https://travis-ci.org/kubernetes/dns.svg?branch=master)](https://travis-ci.org/kubernetes/dns)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes/dns/badge.svg?branch=master)](https://coveralls.io/github/kubernetes/dns?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes/dns)](https://goreportcard.com/report/github.com/kubernetes/dns)

This is the repository for [Kubernetes DNS](http://kubernetes.io/docs/admin/dns/).

## Images

* [kube-dns](http://kubernetes.io/docs/admin/dns/)
* [sidecar](docs/sidecar/README.md)
* [dnsmasq](images/dnsmasq)
* [node-cache](https://kubernetes.io/docs/tasks/administer-cluster/nodelocaldns/)

## Building

`make` targets:

| target | description |
| ---- | ---- |
|all, build   | build all binaries |
|test         | run unit tests |
|containers   | build the containers |
|images-clean | clear image build artifacts from workdir |
|push         | push containers to the registry |
|help         | this help message |
|version      | show package version |
|{build,containers,push}-ARCH | do action for specific ARCH |
|all-{build,containers,push}  | do action for all ARCH |
|only-push-BINARY             | push just BINARY |

* Setting `VERBOSE=1` will show additional build logging.
* Setting `VERSION` will override the container version tag.


## Release process

1. Build and test (`make images-clean`; `make build`; `make containers`; `make test`)
2. The same steps are executed via the presubmit script `presubmits.sh` which is run by the [test-infra prow job.](https://github.com/kubernetes/test-infra/blob/88cd2798f36010e071a30c9827f90e647b59fc65/config/jobs/kubernetes/sig-network/sig-network-misc.yaml#L182)
3. Update [go dependencies](docs/go-dependencies.md) if needed.
4. Update the release tag. We use [semantic versioning](http://semver.org) to
   name releases.
4. Wait for container images to be pushed via cloudbuild yaml. This will be done automatically by
   `k8s.io/test-infra/.../k8s-staging-dns.yaml`. A manual cloud build can be submitted via
   `gcloud builds submit --config cloudbuild.yaml`, but this requires owner permissions in k8s-staging-dns project.
   The automated job pushes images for all architectures and makes them available in `gcr.io/k8s-staging-dns`.
   Status for build jobs can be checked at - https://k8s-testgrid.appspot.com/sig-network-dns#dns-push-images
5. Promote the images to `gcr.io/k8s-artifacts-prod` using the process described
   in [this](https://github.com/kubernetes/k8s.io/tree/main/k8s.gcr.io#image-promoter) link.
   The image SHAs should be added to `images/k8s-staging-dns/images.yaml`.
6. Submit a PR for the kubernetes/kubernetes repository to switch to the new
   version of the containers.
7. Images will be available in the repo k8s.gcr.io/dns/. The node-cache image with tag 1.15.14 can be found at k8s.gcr.io/dns/k8s-dns-node-cache:1.15.14. Older versions are at k8s.gcr.io/k8s-dns-node-cache:<TAG>
