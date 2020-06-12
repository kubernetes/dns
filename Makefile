# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# `make help` will show commonly used targets.
#

# Golang package.
PKG := k8s.io/dns

# List of binaries to build.
BINARIES := \
    e2e \
    ginkgo \
    sidecar-e2e

# List of binaries to build that are containerized and pushed.
# You must have a matching Dockerfile.BINARY for each BINARY.
CONTAINER_BINARIES := \
    dnsmasq-nanny \
    kube-dns \
    node-cache \
    sidecar

# List of images to build (contained in images/)
IMAGES := dnsmasq
# Registry to push to.
REGISTRY ?= gcr.io/k8s-staging-dns
# Default architecture to build for.
ARCH ?= amd64
# Image to use for building.
BUILD_IMAGE ?= golang:1.14-alpine
# Containers will be named: $(CONTAINER_PREFIX)-$(BINARY)-$(ARCH):$(VERSION)
CONTAINER_PREFIX ?= k8s-dns

# This version-strategy uses git tags to set the version string
VERSION ?= $(shell git describe --tags --always --dirty)

# Set to 1 to print more verbose output from the build.
VERBOSE ?= 0

# Include standard build rules.
include rules.mk

# Additional rule to ensure that the dnsmasq image is built before the
# dnsmasq-nanny image.
BINARY := dnsmasq-nanny
.$(BUILDSTAMP_NAME)-container: images-containers
