#!/bin/bash

# Exit at the first failure.
set -e
# These are the commands run by the prow presubmit job.

echo "installing ginkgo"
mkdir -p ${GOPATH}/src/k8s.io
ln -s `pwd` ${GOPATH}/src/k8s.io/dns
GOFLAGS="-mod=vendor" go install github.com/onsi/ginkgo/ginkgo
export PATH=$PATH:$HOME/gopath/bin

echo "installing sudo"
apt-get update && apt-get install sudo -y

make build
make test
make all-containers
bash test/e2e/sidecar/e2e.sh
sudo -v
ginkgo test/e2e

