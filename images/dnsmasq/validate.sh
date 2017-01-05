#!/bin/bash

if [ -z "$IMAGE" ]; then
  echo "IMAGE needs to be set to the dnsmasq image"
fi

echo "Checking ${IMAGE} ${ARCH}"

# Check that dnsmasq is able to start.
exec docker run --rm -- "${IMAGE}" --version >/dev/null
