#!/usr/bin/env bash

# Copyright 2018 The Kubernetes Authors.
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

# This script will be run bazel when building process starts to
# generate key-value information that represents the status of the
# workspace. The output should be like
#
# KEY1 VALUE1
# KEY2 VALUE2
#
# If the script exits with non-zero code, it's considered as a failure
# and the output will be discarded.

if [[ -z "${DOCKER_REGISTRY}" ]]; then
  DOCKER_REGISTRY="gcr.io"
fi

if [[ -z "${DOCKER_IMAGE_PREFIX}" ]]; then
  DOCKER_IMAGE_PREFIX=`gcloud config get-value project`/
fi

if [[ -z "${DOCKER_TAG}" ]]; then
  DOCKER_TAG="latest"
fi

echo "STABLE_DOCKER_REGISTRY ${DOCKER_REGISTRY}"
echo "STABLE_DOCKER_IMAGE_PREFIX ${DOCKER_IMAGE_PREFIX}"
echo "STABLE_DOCKER_TAG ${DOCKER_TAG}"

if [[ -z "${K8S_CLUSTER}" ]]; then
  K8S_CLUSTER=`kubectl config current-context`
fi

echo "STABLE_K8S_CLUSTER ${K8S_CLUSTER}"
