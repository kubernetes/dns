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
export CONTAINER_PREFIX
export IMAGES
export REGISTRY
export VERBOSE
export VERSION

# directories which hold app source (not vendored)
SRC_DIRS := cmd pkg

ALL_ARCH := amd64 arm arm64 ppc64le s390x
ML_PLATFORMS=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x
# Set default base image dynamically for each arch
ifeq ($(ARCH),amd64)
    BASEIMAGE?=busybox
endif
ifeq ($(ARCH),arm)
    BASEIMAGE?=armel/busybox
endif
ifeq ($(ARCH),arm64)
    BASEIMAGE?=aarch64/busybox
endif
ifeq ($(ARCH),ppc64le)
    BASEIMAGE?=ppc64le/busybox
endif
ifeq ($(ARCH),s390x)
    BASEIMAGE?=s390x/busybox
endif

# These rules MUST be expanded at reference time (hence '=') as BINARY
# is dynamically scoped.
CONTAINER_NAME  = $(REGISTRY)/$(CONTAINER_PREFIX)-$(BINARY)-$(ARCH)
BUILDSTAMP_NAME = $(subst /,_,$(CONTAINER_NAME))_$(VERSION)

CONTAINER_NAME_UNSUFFIXED  = $(REGISTRY)/$(CONTAINER_PREFIX)-$(BINARY)
BUILDSTAMP_NAME_UNSUFFIXED = $(subst /,_,$(CONTAINER_NAME_UNSUFFIXED))_$(VERSION)


GO_BINARIES := $(addprefix bin/$(ARCH)/,$(BINARIES))
CONTAINER_BUILDSTAMPS := $(foreach BINARY,$(BINARIES),.$(BUILDSTAMP_NAME)-container)
PUSH_BUILDSTAMPS := $(foreach BINARY,$(BINARIES),.$(BUILDSTAMP_NAME)-push)
PUSH_ML_BUILDSTAMPS := $(foreach BINARY,$(BINARIES),.$(BUILDSTAMP_NAME_UNSUFFIXED)-pushml)

ifeq ($(VERBOSE),1)
	DOCKER_BUILD_FLAGS :=
	VERBOSE_OUTPUT := >&1
else
	DOCKER_BUILD_FLAGS := -q
	VERBOSE_OUTPUT := >/dev/null
endif

all: build

build-%:
	@$(MAKE) --no-print-directory ARCH=$* build

containers-%:
	@$(MAKE) --no-print-directory ARCH=$* containers

push-%:
	@$(MAKE) --no-print-directory ARCH=$* push


.PHONY: all-build
all-build: $(addprefix build-,$(ALL_ARCH)) images-build

.PHONY: all-containers
all-containers: $(addprefix containers-,$(ALL_ARCH)) images-containers

.PHONY: all-push
all-push: ./manifest-tool gcr-login $(addprefix push-,$(ALL_ARCH)) pushml images-push

.PHONY: build
build: $(GO_BINARIES)

./manifest-tool:
	curl -sSL https://github.com/luxas/manifest-tool/releases/download/v0.3.0/manifest-tool > manifest-tool
	chmod +x manifest-tool

gcr-login:
ifeq ($(findstring gcr.io,$(PREFIX)),gcr.io)
	@echo "If you are pushing to a gcr.io registry, you have to be logged in via 'docker login'; 'gcloud docker push' can't push manifest lists yet."
	@echo "This script is automatically logging you in now."
	docker login -u oauth2accesstoken -p "$(gcloud auth print-access-token)" https://gcr.io
endif

# Rule for all bin/$(ARCH)/bin/$(BINARY)
$(GO_BINARIES): build-dirs
	@echo "building : $@"
	@docker run                                                            \
	    --rm                                                               \
	    --sig-proxy=true                                                   \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin                                     \
	    -v $$(pwd)/bin/$(ARCH):/go/bin/linux_$(ARCH)                       \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
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
	    -e 's|ARG_BIN|$(BINARY)|g'		\
	    -e 's|ARG_ARCH|$(ARCH)|g'		\
	    -e 's|ARG_FROM|$(BASEIMAGE)|g'	\
	    $$< > $$@
.$(BUILDSTAMP_NAME)-container: .$(BINARY)-$(ARCH)-dockerfile
endef
$(foreach BINARY,$(BINARIES),$(eval $(DOCKERFILE_RULE)))


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
$(foreach BINARY,$(BINARIES),$(eval $(CONTAINER_RULE)))

.PHONY: containers
containers: $(CONTAINER_BUILDSTAMPS)


# Rules for pushing
.PHONY: push
push: $(PUSH_BUILDSTAMPS)

.%-push: .%-container
	@echo "pushing  :" $$(head -n 1 $<)
	@docker push $$(head -n 1 $<) $(VERBOSE_OUTPUT)
	@cat $< > $@

define PUSH_RULE
only-push-$(BINARY): .$(BUILDSTAMP_NAME)-push
endef
$(foreach BINARY,$(BINARIES),$(eval $(PUSH_RULE)))


# Rules for pushing manifest lists
.PHONY: pushml
pushml: $(PUSH_ML_BUILDSTAMPS)

define PUSHML_RULE
.$(BUILDSTAMP_NAME_UNSUFFIXED)-pushml:
	@echo "pushing manifest list :" $(CONTAINER_NAME_UNSUFFIXED):$(VERSION)
	@./manifest-tool push from-args \
		--platforms $(ML_PLATFORMS) \
		--template $(CONTAINER_NAME_UNSUFFIXED)-ARCH:$(VERSION) \
		--target $(CONTAINER_NAME_UNSUFFIXED):$(VERSION) \
		$(VERBOSE_OUTPUT)
	@echo "$(CONTAINER_NAME_UNSUFFIXED):$(VERSION)" > $$@
endef
$(foreach BINARY,$(BINARIES),$(eval $(PUSHML_RULE)))


# Rule for `test`
.PHONY: test
test: build-dirs
	@docker run                                                            \
	    --rm                                                               \
	    --sig-proxy=true                                                   \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin                                     \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
	    -w /go/src/$(PKG)                                                  \
	    $(BUILD_IMAGE)                                                     \
	    /bin/sh -c "                                                       \
	        ./build/test.sh $(SRC_DIRS)                                    \
	    "

# Hook in images build
.PHONY: images-build
images-build:
	@$(MAKE) -C images all-build

.PHONY: images-containers
images-containers:
	@$(MAKE) -C images all-containers

.PHONY: images-push
images-push:
	@$(MAKE) -C images all-push

.PHONY: images-test
images-test:
	@$(MAKE) -C images all-test

.PHONY: images-clean
images-clean:
	@$(MAKE) -C images all-clean

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
	@echo "  help          this help message"
	@echo "  version       show package version"
	@echo
	@echo "  {build,containers,push}-ARCH    do action for specific ARCH"
	@echo "  all-{build,containers,push}     do action for all ARCH"
	@echo "  only-push-BINARY                push just BINARY"
	@echo
	@echo "  Available ARCH: $(ALL_ARCH)"
	@echo "  Available BINARIES: $(BINARIES)"
	@echo
	@echo "  Setting VERBOSE=1 will show additional build logging."
	@echo
	@echo "  Setting VERSION will override the container version tag."
