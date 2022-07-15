#!/bin/bash
set -e

if [ -z "$IMAGE" ]; then
  echo "IMAGE needs to be set to the dnsmasq image"
fi

echo "Checking ${IMAGE} ${ARCH}"

# Check that dnsmasq is correctly compiled and has the right
# dynamic libraries to run.
python3 validate_dynamic_deps.py --image ${IMAGE} --target-bin /usr/sbin/dnsmasq

# Check that dnsmasq is able to start.
exec docker run --rm -- "${IMAGE}" --version >/dev/null

