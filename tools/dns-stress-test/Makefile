PROJECT := $(shell gcloud config get-value project)
DOCKER_IMAGE_PREFIX := ${PROJECT}/

VARIANT := $(shell kubectl config current-context)

.PHONY: all
all: build test

.PHONY: build
build:
	bazel build //...

.PHONY: test
test:
	bazel test //...

.PHONY: gazelle
gazelle:
	bazel run //:gazelle

.PHONY: update-deps
update-deps:
	bazel run //:gazelle -- update-repos -from_file=go.mod
