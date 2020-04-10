#!/bin/bash
# This script runs some very basic commands to ensure that the newly build
# images are working correctly. Invoke as:
# ./image-checks.sh <image-tag> <registry-name>
# Kill with Ctrl + C once sidecar starts up successfully.
TAG=$1
REGISTRY=${2:-gcr.io/google-containers}
echo "Verifying that iptables exists in node-cache image"
docker run --rm -it --entrypoint=iptables ${REGISTRY}/k8s-dns-node-cache:${TAG}
echo "Verifying that node-cache binary exists in node-cache image"
docker run --rm -it --entrypoint=/node-cache ${REGISTRY}/k8s-dns-node-cache:${TAG}
echo "Verifying dnsmasq-nanny startup"
docker run --rm -it --entrypoint=/dnsmasq-nanny ${REGISTRY}/k8s-dns-dnsmasq-nanny:${TAG}
echo "Verifying kube-dns startup"
docker run --rm -it --entrypoint=/kube-dns ${REGISTRY}/k8s-dns-kube-dns:${TAG}
echo "Verifying sidecar startup"
docker run --rm -it --entrypoint=/sidecar ${REGISTRY}/k8s-dns-sidecar:${TAG}
