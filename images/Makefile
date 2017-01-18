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

# Each image/ Makefile MUST provide the following targets:
# 
# build      : build the source code
# containers : create the Docker containers
# test       : test the containers
# push       : push the containers to the registry
#
# The following variables are exported to the submake process:
#
# REGISTRY : registry to push to
# ARCH     : architecture to build
# CONTAINER_PREFIX: prefix for container names
# VERSION  : version, e.g. tag for the containers
# VERBOSE  : will be 1 if running in verbose mode

# List of image directories to build.
IMAGES ?= $(shell ls -d */)

all: build

.PHONY: build
all-build:
	@$(MAKE) $(addprefix all-build-,$(IMAGES))

.PHONY: containers
all-containers:
	@$(MAKE) $(addprefix all-containers-,$(IMAGES))

.PHONY: push
all-push:
	@$(MAKE) $(addprefix all-push-,$(IMAGES))

.PHONY: test
all-test:
	@$(MAKE) $(addprefix all-test-,$(IMAGES))

.PHONY: clean
all-clean:
	@$(MAKE) $(addprefix all-clean-,$(IMAGES))

# Suffix rules to dispatch to the appropriate subdirectory.
all-build-%:
	@$(MAKE) -C $* all-build

all-containers-%:
	@$(MAKE) -C $* all-containers

all-push-%:
	@$(MAKE) -C $* all-push

all-test-%:
	@$(MAKE) -C $* all-test

all-clean-%:
	@$(MAKE) -C $* all-clean
