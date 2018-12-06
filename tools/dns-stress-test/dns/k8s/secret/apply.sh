#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

cd "$(dirname "$0")"

kubectl create secret generic dns-stress-test --from-file key.json --dry-run  -oyaml | kubectl apply -n dns-stress-test -f -
