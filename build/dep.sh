#!/bin/sh
# Copyright 2017 The Kubernetes Authors.
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

set -e

IMAGE="k8s-dns-godep"
GOLANG_IMAGE="golang:1.9.3-alpine"
DNS_SRC="/go/src/k8s.io/dns"
REQUIRED_PKGS="github.com/onsi/ginkgo/ginkgo/... golang.org/x/text/... ./pkg/... ./cmd/..."

OSNAME=$(uname -s)
if [ ${OSNAME} = "Darwin" ]; then
    USERGROUP=$(stat -f '%u:%g' build/dep.sh)
else
    USERGROUP=$(stat -c '%u:%g' build/dep.sh)
fi
   
TMPDIR=$(mktemp -d)
trap "rm -rf ${TMPDIR}" EXIT

image() {
  local NO_CACHE=""
  if [ "$1" = "--no-cache" ]; then
    NO_CACHE="--no-cache"
  fi

  docker rmi -f ${IMAGE} || true
  docker rmi -f ${IMAGE}-base || true

  # All docker build materials must be contained in the tree that the
  # Dockerfile is contained in.
  cat <<END |  docker build ${NO_CACHE} -t ${IMAGE}-base -
FROM ${GOLANG_IMAGE}
RUN apk update
RUN apk add git mercurial
RUN touch /.k8s-dns-godep
RUN go get -u github.com/tools/godep
END
  docker run                                    \
    --sig-proxy=true                            \
    --volume=`pwd`:${DNS_SRC}                   \
    --entrypoint sh                             \
    --workdir ${DNS_SRC}                        \
    ${IMAGE}-base                               \
    ${DNS_SRC}/build/dep.sh _setupImage

  local HASH=$(docker ps -q -n 1)
  docker commit ${HASH} ${IMAGE}
}

enter() {
  _checkImage

  local PREV=$(docker images ${IMAGE} -q)
  docker run                                    \
    -it                                         \
    --volume=`pwd`:${DNS_SRC}                   \
    --workdir ${DNS_SRC}                        \
    --entrypoint sh                             \
    ${IMAGE}

  if [ "$1" = "-u" ]; then
    echo "${IMAGE} updated (previous image was ${PREV})"
    local HASH=$(docker ps -q -n 1)
    docker commit ${HASH} ${IMAGE}
  fi

  docker run                                    \
    --sig-proxy=true                            \
    --volume=`pwd`:${DNS_SRC}                   \
    --workdir ${DNS_SRC}                        \
    --entrypoint sh                             \
    ${IMAGE}                                    \
    ${DNS_SRC}/build/dep.sh _fixPermissions
}

save() {
  if [ -r /.k8s-dns-godep ]; then
    if [ ! -z "$*" ]; then
      for DEP in $*; do
        echo go get ${DEP}/...
        go get ${DEP}/...
      done
    fi

    echo godep save ${REQUIRED_PKGS}
    eval godep save ${REQUIRED_PKGS}
    _fixPermissions
  else
    _checkImage

    local PREV=$(docker images ${IMAGE} -q)
    # Run save inside the container.
    docker run                                    \
      --sig-proxy=true                            \
      --volume=`pwd`:${DNS_SRC}                   \
      --entrypoint sh                             \
      --workdir ${DNS_SRC}                        \
      ${IMAGE}                                    \
      ${DNS_SRC}/build/dep.sh save "$*"

    echo "${IMAGE} updated (previous image was ${PREV})"
    local HASH=$(docker ps -q -n 1)
    docker commit ${HASH} ${IMAGE}
   fi
}

verify() {
  if [ -r /.k8s-dns-godep ]; then
    cp -r ${DNS_SRC}.orig/* ${DNS_SRC}
    cd ${DNS_SRC}
    rm -rf Godeps/
    eval godep save ${REQUIRED_PKGS}
    cp -r Godeps/ /data
    chown -R ${USERGROUP} /data/Godeps
  else
    _checkImage

    docker run                                  \
    --sig-proxy=true                            \
    --volume=`pwd`:${DNS_SRC}.orig:ro           \
    --volume=${TMPDIR}:/data                    \
    --workdir ${DNS_SRC}.orig                   \
    ${IMAGE}                                    \
    ${DNS_SRC}.orig/build/dep.sh verify "$*"

    diff -u Godeps/Godeps.json ${TMPDIR}/Godeps/Godeps.json
    exit $?
  fi
}

usage() {
  cat <<END
dep.sh COMMAND ...

Manages godep dependencies. This mostly a wrapper around operations
involving godep and a Docker image containing a clean copy of the
dependencies.

image [--no-cache]
  Create image with godep dependencies (${IMAGE}). This must be done
  first before running the rest of the commands.

  If --no-cache is given, then the Docker image is built without caching.

verify
  Verify that the godep matches the dependencies in the code. Exits
  with success if there are no differences.

enter [-u]
  Run Docker container interactively to manage godep. If -u is
  specified, then update the image with changes made.

save [PKG1 PKG2 ...]
  save PKGs as new dependencies. Note: the PKGs must be referenced
  from the code, otherwise godep will ignore the new packages.

-h|--help
  This help message.

USAGE

Add a new package:

  # Add import reference of acme.com/widget.
  \$ vi pkg/foo.go

  # Add widget to the vendor directory. This should modify the godep
  # file appropriately.
  \$ build/dep.sh save acme.com/widget


Updating a package:

  (It is recommended to do this manually due to the fragility of godep update)

  # Enter the container interactively.
  \$ build/dep.sh enter -u

  # inside container
  \$ rm -rf /src/\$DEP # repo root
  \$ go get \$DEP/...
  # Change code in Kubernetes to reference new DEP code, if necessary.
  \$ rm -rf Godeps
  \$ rm -rf vendor
  \$ ./build/dep.sh save
  \$ exit

  \$ git checkout -- \$(git status -s | grep "^ D" | awk '{print \$2}' | grep ^Godeps)

Verifying Godeps.json match:

  \$ build/dep.sh verify

END
}

_setupImage() {
  go get ./...
  godep restore -v
}

_fixPermissions() {
  # Make sure Godeps and vendor permissions match the host
  # (non-container) ids.
  chown -R ${USERGROUP} Godeps/
  chown -R ${USERGROUP} vendor/
}

_checkImage() {
  if [ -z $(docker images k8s-dns-godep -q) ]; then
    echo "Must run `dep.sh image` to first to create godep image"
    exit 1
  fi
}

case $1 in
  ''|-h|--help)
    usage
    exit 0;;

  image)
    shift; image "$*";;
  enter)
    shift; enter "$*";;
  save)
    shift; save "$*";;
  verify)
    shift; verify "$*";;

  _fixPermissions)
    shift; _fixPermissions "$*";;
  _setupImage)
    shift; _setupImage "$*";;

  *)
    echo "Invalid command: $1"
    echo
    usage
    exit 1;;
esac
