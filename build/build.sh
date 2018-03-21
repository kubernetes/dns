#!/bin/sh
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

set -o errexit
set -o nounset
set -o pipefail

if [ -z "${PKG}" ]; then
    echo "PKG must be set"
    exit 1
fi
if [ -z "${ARCH}" ]; then
    echo "ARCH must be set"
    exit 1
fi
if [ -z "${VERSION}" ]; then
    echo "VERSION must be set"
    exit 1
fi

export CGO_ENABLED=0
export GOARCH="${ARCH}"
if [ $GOARCH == "amd64" ]; then
    export GOBIN="$GOPATH/bin/linux_amd64"
fi

# ./vendor/... is specified to include installing binary from vendor folder.
# Since Go 1.9, vendor matching will no longer work with ./...
# (https://golang.org/doc/go1.9#vendor-dotdotdot).
# This is currently required because our travis CI is expecting the ginkgo
# binary. We might get rid of this after removing that dependency.
go install                                                         \
    -installsuffix "static"                                        \
    -ldflags "-X ${PKG}/pkg/version.VERSION=${VERSION}"            \
    ./...                                                          \
    ./vendor/...
