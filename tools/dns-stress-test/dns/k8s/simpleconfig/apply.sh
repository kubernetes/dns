#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

cd "$(dirname "$0")"

kubectl create configmap dns --from-file sites --dry-run  -oyaml | kubectl apply -n dns-stress-test -f -
