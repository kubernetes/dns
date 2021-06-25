#!/bin/bash

# These are the commands run by the prow presubmit job.

#install ginkgo
mkdir -p ${GOPATH}/src/k8s.io
ln -s `pwd` ${GOPATH}/src/k8s.io/dns
GOFLAGS="-mod=vendor" go install github.com/onsi/ginkgo/ginkgo
export PATH=$PATH:$HOME/gopath/bin

make build
make test
make all-containers
bash test/e2e/sidecar/e2e.sh
ginkgo test/e2e

