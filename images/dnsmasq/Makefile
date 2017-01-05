# Copyright 2016 The Kubernetes Authors All rights reserved.
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

VERSION ?= $(shell git describe --tags --always --dirty)
REGISTRY ?= gcr.io/google_containers
ARCH ?= amd64
DNSMASQ_VERSION ?= dnsmasq-2.76
CONTAINER_PREFIX ?= k8s-dns

ALL_ARCH := amd64 arm arm64 ppc64le
IMAGE := $(CONTAINER_PREFIX)-dnsmasq-$(ARCH)
COMPILE_IMAGE := gcr.io/google_containers/kube-cross:v1.6.2-2
OUTPUT_DIR := _output/$(ARCH)

ifeq ($(ARCH),amd64)
	BASEIMAGE ?= alpine:3.4
	COMPILE_IMAGE := alpine:3.4
else ifeq ($(ARCH),arm)
	BASEIMAGE ?= armel/busybox:glibc
	TRIPLE    ?= arm-linux-gnueabi
	QEMUARCH  := arm
else ifeq ($(ARCH),arm64)
	BASEIMAGE ?= aarch64/busybox:glibc
	TRIPLE    ?= aarch64-linux-gnu
	QEMUARCH  := aarch64
else ifeq ($(ARCH),ppc64le)
	BASEIMAGE ?= ppc64le/busybox:glibc
	TRIPLE    ?= powerpc64le-linux-gnu
	QEMUARCH  := ppc64le
else
$(error Unsupported ARCH: $(ARCH))
endif

DNSMASQ_URL := http://www.thekelleys.org.uk/dnsmasq/$(DNSMASQ_VERSION).tar.xz
DNSMASQ_ARCHIVE := $(OUTPUT_DIR)/dnsmasq.tar.xz

MULTIARCH_CONTAINER := multiarch/qemu-user-static:register
MULTIARCH_RELEASE := https://github.com/multiarch/qemu-user-static/releases/download/v2.5.0/x86_64_qemu-$(QEMUARCH)-static.tar.xz

DOCKER := docker
ifeq ($(findstring gcr.io/,$(REGISTRY)),gcr.io/)
	DOCKER := gcloud docker --
endif

BINARY:= $(OUTPUT_DIR)/dnsmasq

BUILDSTAMP := $(subst /,_,$(REGISTRY))_$(IMAGE)_$(VERSION)
CONTAINER_STAMP := .$(BUILDSTAMP)-container
PUSH_STAMP := .$(BUILDSTAMP)-push

ifeq ($(VERBOSE), 1)
	VERBOSE_OUTPUT := >&1
else
	VERBOSE_OUTPUT := >/dev/null 2>/dev/null
endif

all: build

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

containers-%:
	@$(MAKE) --no-print-directory ARCH=$* containers

test-%:
	@$(MAKE) --no-print-directory ARCH=$* test

push-%:
	@$(MAKE) --no-print-directory ARCH=$* push

.PHONY: all-build
all-build: $(addprefix build-, $(ALL_ARCH))

.PHONY: all-containers
all-containers: $(addprefix containers-, $(ALL_ARCH))

.PHONY: all-test
all-test: $(addprefix test-, $(ALL_ARCH))

.PHONY: all-push
all-push: $(addprefix push-, $(ALL_ARCH))

.PHONY: build
build: $(BINARY)

$(BINARY): Dockerfile.cross $(DNSMASQ_ARCHIVE) dnsmasq.conf
	@echo "building :" $(BINARY)
	@mkdir -p $(@D)
	@cp Dockerfile.cross $(OUTPUT_DIR)/Dockerfile
	@cd $(OUTPUT_DIR) && sed -i "s|__BASEIMAGE__|$(BASEIMAGE)|g" Dockerfile

ifeq ($(ARCH),amd64)
	@cd $(OUTPUT_DIR) && sed -i "/__CROSS_BUILD_COPY__/d" Dockerfile
	@docker run --rm --sig-proxy=true     \
		-v `pwd`/$(OUTPUT_DIR):/build \
		-v `pwd`:/src                 \
		$(COMPILE_IMAGE)              \
		/bin/sh -c                    \
		"apk update                                 \
			&& apk add alpine-sdk xz libcap-dev \
			&& tar -xJf /build/dnsmasq.tar.xz   \
			&& cd $(DNSMASQ_VERSION)            \
			&& make -j                          \
			&& cp src/dnsmasq /build" $(VERBOSE_OUTPUT)
else
	@cd $(OUTPUT_DIR)                                            \
		&& sed -i "s|__ARCH__|$(QEMUARCH)|g" Dockerfile      \
		&& sed -i "s/__CROSS_BUILD_COPY__/COPY/g" Dockerfile
	@cd $(OUTPUT_DIR) \
		&& curl -sSL $(MULTIARCH_RELEASE) | tar -xJ
	@docker run --sig-proxy=true --rm            \
		--privileged $(MULTIARCH_CONTAINER)  \
		--reset $(VERBOSE_OUTPUT)
	@docker run --rm --sig-proxy=true            \
		-v `pwd`/$(OUTPUT_DIR):/build        \
		-v `pwd`:/src                        \
		$(COMPILE_IMAGE)                     \
		/bin/bash -c                         \
		"tar -xJf /build/dnsmasq.tar.xz      \
			&& cd $(DNSMASQ_VERSION)     \
			&& CC=$(TRIPLE)-gcc make -j  \
			&& cp src/dnsmasq /build" $(VERBOSE_OUTPUT)
endif
	@cp dnsmasq.conf $(OUTPUT_DIR)

$(DNSMASQ_ARCHIVE):
	@mkdir -p $(@D)
	@curl -sSL $(DNSMASQ_URL) > $@

.PHONY: containers
containers: $(CONTAINER_STAMP)

$(CONTAINER_STAMP): $(BINARY)
	@echo "container:" $(REGISTRY)/$(IMAGE):$(VERSION)
	@docker build \
		-q -t $(REGISTRY)/$(IMAGE):$(VERSION) $(OUTPUT_DIR) > $@

.PHONY: test
test: containers
	@ARCH=$(ARCH) IMAGE=$(REGISTRY)/$(IMAGE):$(VERSION) ./validate.sh


.PHONY: push
push: $(PUSH_STAMP)

$(PUSH_STAMP): $(CONTAINER_STAMP)
	@echo "pushing  :" $(REGISTRY)/$(IMAGE):$(VERSION)
	@$(DOCKER) push $(REGISTRY)/$(IMAGE):$(VERSION)
	@cat $< > $@

.PHONY: clean
clean:
	rm -f .*-container
	rm -f .*-push
	rm -rf _output/

.PHONY: version
version:
	@echo $(VERSION)
