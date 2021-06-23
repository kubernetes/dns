#!/bin/bash

# These are the commands run by the prow presubmit job.

service docker start
make test VERBOSE=5
