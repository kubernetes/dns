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

ALL_ARCH := amd64 arm arm64 ppc64le s390x
IMAGENAME := $(CONTAINER_PREFIX)-dnsmasq
# TODO: Use gcr.io/google_containers/kube-cross:v1.7.4-2
COMPILE_IMAGE := gcr.io/google_containers/kube-cross:v1.7.4-1
OUTPUT_DIR := _output/$(ARCH)
ML_PLATFORMS=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x

ifeq ($(ARCH),amd64)
	BASEIMAGE ?= busybox:glibc
	CC ?= gcc
else ifeq ($(ARCH),arm)
	# TODO: Switch to using armhf/ and arm-linux-gnueabihf
	BASEIMAGE ?= armel/busybox:glibc
	CC        ?= arm-linux-gnueabi-gcc
else ifeq ($(ARCH),arm64)
	BASEIMAGE ?= aarch64/busybox:glibc
	CC        ?= aarch64-linux-gnu-gcc
else ifeq ($(ARCH),ppc64le)
	BASEIMAGE ?= ppc64le/busybox:glibc
	CC        ?= powerpc64le-linux-gnu-gcc
else ifeq ($(ARCH),s390x)
	BASEIMAGE ?= s390x/busybox:glibc
	CC        ?= s390x-linux-gnu-gcc
else
$(error Unsupported ARCH: $(ARCH))
endif

DNSMASQ_URL := http://www.thekelleys.org.uk/dnsmasq/$(DNSMASQ_VERSION).tar.xz
BINARY:= $(OUTPUT_DIR)/dnsmasq

BUILDSTAMP := $(subst /,_,$(REGISTRY))_$(IMAGENAME)-$(ARCH)_$(VERSION)
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
all-push: ./manifest-tool $(addprefix push-, $(ALL_ARCH))
	./manifest-tool push from-args --platforms $(ML_PLATFORMS) --template $(REGISTRY)/$(IMAGENAME)-ARCH:$(VERSION) --target $(REGISTRY)/$(IMAGENAME):$(VERSION)

.PHONY: build
build: $(BINARY)

$(BINARY):
	@echo "building :" $(BINARY)
	@mkdir -p $(@D)
	@docker run --rm --sig-proxy=true        \
		-v `pwd`/$(OUTPUT_DIR):/build        \
		$(COMPILE_IMAGE)                     \
		/bin/bash -c                         \
		"curl -sSL $(DNSMASQ_URL) | tar -xJ  \
			&& cd $(DNSMASQ_VERSION)     \
			&& CC=$(CC) make -j  \
			&& cp src/dnsmasq /build" $(VERBOSE_OUTPUT)

.PHONY: containers
containers: $(CONTAINER_STAMP)

$(CONTAINER_STAMP): Dockerfile dnsmasq.conf $(BINARY)
	@echo "container:" $(REGISTRY)/$(IMAGENAME)-$(ARCH):$(VERSION)
	@cp dnsmasq.conf $(OUTPUT_DIR)/
	@sed "s|__BASEIMAGE__|$(BASEIMAGE)|g" Dockerfile > $(OUTPUT_DIR)/Dockerfile
	@touch $(OUTPUT_DIR)/.emptydir
	@docker build \
		-q -t $(REGISTRY)/$(IMAGENAME)-$(ARCH):$(VERSION) $(OUTPUT_DIR) > $@

.PHONY: test
test: containers
	@ARCH=$(ARCH) IMAGE=$(REGISTRY)/$(IMAGENAME)-$(ARCH):$(VERSION) ./validate.sh

./manifest-tool:
	curl -sSL https://github.com/luxas/manifest-tool/releases/download/v0.3.0/manifest-tool > manifest-tool
	chmod +x manifest-tool

gcr-login:
ifeq ($(findstring gcr.io,$(PREFIX)),gcr.io)
	@echo "If you are pushing to a gcr.io registry, you have to be logged in via 'docker login'; 'gcloud docker push' can't push manifest lists yet."
	@echo "This script is automatically logging you in now."
	docker login -u oauth2accesstoken -p "$(gcloud auth print-access-token)" https://gcr.io
endif

.PHONY: push
push: $(PUSH_STAMP)

$(PUSH_STAMP): $(CONTAINER_STAMP) gcr-login
	@echo "pushing  :" $(REGISTRY)/$(IMAGENAME)-$(ARCH):$(VERSION)
	@docker push $(REGISTRY)/$(IMAGENAME)-$(ARCH):$(VERSION)
	@cat $< > $@

.PHONY: clean
clean:
	rm -f .*-container
	rm -f .*-push
	rm -rf _output/

.PHONY: version
version:
	@echo $(VERSION)
