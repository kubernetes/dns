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
# These build rules should not need to be modified.
#

# Variables exported to submake
export ARCH
export MANIFEST_IMAGE
export CONTAINER_PREFIX
export IMAGES
export REGISTRY
export VERBOSE
export VERSION

# directories which hold app source (not vendored)
SRC_DIRS := cmd pkg

ALL_ARCH := amd64 arm arm64 ppc64le s390x
BASEIMAGE ?= k8s.gcr.io/build-image/debian-base-$(ARCH):bullseye-v1.3.0
IPTIMAGE ?= k8s.gcr.io/build-image/debian-iptables-$(ARCH):bullseye-v1.4.0

# These rules MUST be expanded at reference time (hence '=') as BINARY
# is dynamically scoped.
CONTAINER_NAME  = $(REGISTRY)/$(CONTAINER_PREFIX)-$(BINARY)-$(ARCH)
BUILDSTAMP_NAME = $(subst :,_,$(subst /,_,$(CONTAINER_NAME))_$(VERSION))

# Ensure that the docker command line supports the manifest images
export DOCKER_CLI_EXPERIMENTAL=enabled

ALL_BINARIES += $(BINARIES)
ALL_BINARIES += $(CONTAINER_BINARIES)

GO_BINARIES := $(addprefix bin/$(ARCH)/,$(ALL_BINARIES))
CONTAINER_BUILDSTAMPS := $(foreach BINARY,$(CONTAINER_BINARIES),.$(BUILDSTAMP_NAME)-container)
PUSH_BUILDSTAMPS := $(foreach BINARY,$(CONTAINER_BINARIES),.$(BUILDSTAMP_NAME)-push)

ifeq ($(VERBOSE), 1)
	DOCKER_BUILD_FLAGS :=
	VERBOSE_OUTPUT := >&1
else
	DOCKER_BUILD_FLAGS := -q
	VERBOSE_OUTPUT := >/dev/null
endif

# This MUST appear as the first rule in a Makefile
all: build

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

containers-%:
	@$(MAKE) --no-print-directory ARCH=$* containers

push-%:
	@$(MAKE) --no-print-directory ARCH=$* push


.PHONY: all-build
all-build: $(addprefix build-, $(ALL_ARCH))

.PHONY: all-containers
all-containers: $(addprefix containers-, $(ALL_ARCH))

.PHONY: all-push
all-push: $(addprefix push-, $(ALL_ARCH))
	@for binary in $(CONTAINER_BINARIES); do \
		MANIFEST_IMAGE=$(REGISTRY)/$(CONTAINER_PREFIX)-$${binary} ; \
		for arch in $(ALL_ARCH); do \
			docker manifest create --amend $$MANIFEST_IMAGE:$(VERSION) $$MANIFEST_IMAGE-$${arch}:${VERSION} ; \
			docker manifest annotate --arch $${arch} $$MANIFEST_IMAGE:${VERSION} $$MANIFEST_IMAGE-$${arch}:${VERSION}; \
		done ; \
		docker manifest push --purge $$MANIFEST_IMAGE:${VERSION} ; \
	done

.PHONY: build
build: $(GO_BINARIES) images-build


# Rule for all bin/$(ARCH)/bin/$(BINARY)
# Add line
# `           -v $(GOCACHE):$(GOCACHE)                                           \`
# to use GOCACHE. Not used currently due to permission issues in  dev setup.
# We also want a clean build in the CI, without caching. GOCACHE env variable cannot
# be set to off in go1.12 and later - https://github.com/golang/go/issues/29378
# So this is a workaround where we set GOCACHE env variable, but do not use it as a volume.
$(GO_BINARIES): build-dirs
	@echo "building : $@"
	@docker pull $(BUILD_IMAGE)
	@docker run                                                            \
	    --rm                                                               \
	    --sig-proxy=true                                                   \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin/linux_$(ARCH)                       \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
	    -e GOCACHE=$(GOCACHE)                                              \
	    -w /go/src/$(PKG)                                                  \
	    $(BUILD_IMAGE)                                                     \
	    /bin/sh -c "                                                       \
	        ARCH=$(ARCH)                                                   \
	        VERSION=$(VERSION)                                             \
	        PKG=$(PKG)                                                     \
	        ./build/build.sh                                               \
	    "


# Rules for dockerfiles.
define DOCKERFILE_RULE
.$(BINARY)-$(ARCH)-dockerfile: Dockerfile.$(BINARY)
	@echo generating Dockerfile $$@ from $$<
	@sed					\
	    -e 's|ARG_ARCH|$(ARCH)|g' \
	    -e 's|ARG_BIN|$(BINARY)|g' \
	    -e 's|ARG_REGISTRY|$(REGISTRY)|g' \
	    -e 's|ARG_FROM_BASE|$(BASEIMAGE)|g' \
	    -e 's|ARG_FROM_IPT|$(IPTIMAGE)|g' \
	    -e 's|ARG_VERSION|$(VERSION)|g' \
	    $$< > $$@
.$(BUILDSTAMP_NAME)-container: .$(BINARY)-$(ARCH)-dockerfile
endef
$(foreach BINARY,$(CONTAINER_BINARIES),$(eval $(DOCKERFILE_RULE)))


# Rules for containers
define CONTAINER_RULE
.$(BUILDSTAMP_NAME)-container: bin/$(ARCH)/$(BINARY)
	@echo "container: bin/$(ARCH)/$(BINARY) ($(CONTAINER_NAME))"
	@docker build					\
		$(DOCKER_BUILD_FLAGS)			\
		-t $(CONTAINER_NAME):$(VERSION)		\
		-f .$(BINARY)-$(ARCH)-dockerfile .	\
		$(VERBOSE_OUTPUT)
	@echo "$(CONTAINER_NAME):$(VERSION)" > $$@
	@docker images -q $(CONTAINER_NAME):$(VERSION) >> $$@
endef
$(foreach BINARY,$(CONTAINER_BINARIES),$(eval $(CONTAINER_RULE)))

.PHONY: containers
containers: $(CONTAINER_BUILDSTAMPS) images-containers


# Rules for pushing
.PHONY: push
push: $(PUSH_BUILDSTAMPS) images-push

.%-push: .%-container
	@echo "pushing  :" $$(head -n 1 $<)
	@docker push $$(head -n 1 $<) $(VERBOSE_OUTPUT)
	@cat $< > $@

define PUSH_RULE
only-push-$(BINARY): .$(BUILDSTAMP_NAME)-push
endef
$(foreach BINARY,$(CONTAINER_BINARIES),$(eval $(PUSH_RULE)))


# Rule for `test`
# Add line
# `           -v $(GOCACHE):$(GOCACHE)                                           \`
# to use GOCACHE. Not used currently due to permission issues in  dev setup.
# We also want a clean build in the CI, without caching. GOCACHE env variable cannot
# be set to off in go1.12 and later - https://github.com/golang/go/issues/29378
# So this is a workaround where we set GOCACHE env variable, but do not use it as a volume.
.PHONY: test
test: build-dirs images-test
	@docker run                                                            \
	    --rm                                                               \
	    --sig-proxy=true                                                   \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin                                     \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
	    -e GOCACHE=$(GOCACHE)                                              \
	    -w /go/src/$(PKG)                                                  \
	    $(BUILD_IMAGE)                                                     \
	    /bin/sh -c "                                                       \
	        ./build/test.sh $(SRC_DIRS)                                    \
	    "

# Hook in images build
.PHONY: images-build
images-build:
	@$(MAKE) -C images build

.PHONY: images-containers
images-containers:
	@$(MAKE) -C images containers

.PHONY: images-push
images-push:
	@$(MAKE) -C images push

.PHONY: images-test
images-test:
	@$(MAKE) -C images test

.PHONY: images-clean
images-clean:
	@$(MAKE) -C images clean

# Miscellaneous rules
.PHONY: version
version:
	@echo $(VERSION)

.PHONY: build-dirs
build-dirs:
	@mkdir -p bin/$(ARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(ARCH)

.PHONY: clean
clean: container-clean bin-clean images-clean

.PHONY: container-clean
container-clean:
	rm -f .*-container .*-dockerfile .*-push

.PHONY: bin-clean
bin-clean:
	rm -rf .go bin

.PHONY: help
help:
	@echo "make targets"
	@echo
	@echo "  all, build    build all binaries"
	@echo "  containers    build the containers"
	@echo "  push          push containers to the registry"
	@echo "  images-clean  clear image build artifacts from workdir"
	@echo "  help          this help message"
	@echo "  version       show package version"
	@echo
	@echo "  {build,containers,push}-ARCH    do action for specific ARCH"
	@echo "  all-{build,containers,push}     do action for all ARCH"
	@echo "  only-push-BINARY                push just BINARY"
	@echo
	@echo "  Available ARCH: $(ALL_ARCH)"
	@echo "  Available BINARIES: $(ALL_BINARIES)"
	@echo
	@echo "  Setting VERBOSE=1 will show additional build logging."
	@echo
	@echo "  Setting VERSION will override the container version tag."
