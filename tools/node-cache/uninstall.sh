#!/bin/bash

set -ex
set -o pipefail

cat nodelocaldns.yaml \
  | kubectl delete -f -
