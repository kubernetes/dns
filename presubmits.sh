#!/bin/bash

# These are the commands run by the prow presubmit job.

make build
make test
make all-containers
bash test/e2e/sidecar/e2e.sh

