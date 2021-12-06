#!/usr/bin/env python

# Copyright 2021 The Kubernetes Authors.
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
# This script pulls the docker images for dns containers and outputs the sha256
# digests for each of the supported arch images.
# These will be used to promote the images to k8s.gcr.io.
import argparse
import json
import subprocess

REGISTRY = "gcr.io/k8s-staging-dns"
MANIFESTS = ["k8s-dns-dnsmasq-nanny", "k8s-dns-kube-dns", "k8s-dns-sidecar", "k8s-dns-node-cache"]
TAG_NAME = "1.21.1"


def main():
    parser = argparse.ArgumentParser(description="Display sha256 for DNS images to be promoted.")
    parser.add_argument("tagname", nargs="?", default=TAG_NAME)
    parser.add_argument("registry", nargs="?", default=REGISTRY)
    args = parser.parse_args()
    for m in MANIFESTS:
        mname = "%s/%s:%s" % (args.registry, m, args.tagname)
        out, err = subprocess.Popen(["docker", "pull", mname], stdout=subprocess.PIPE).communicate()
        outstr = out.decode('UTF-8')
        if err or "Error" in outstr:
            print("Docker pull hit error - %s, %s" % (err, outstr))
            sys.exit(-1)
        maincmd = ["docker", "inspect", "--format='{{index .RepoDigests 0}}'", mname]
        out, err = subprocess.Popen(maincmd, stdout=subprocess.PIPE).communicate()
        # output will be of the form -
        # `gcr.io/k8s-staging-dns/k8s-dns-node-cache@sha256:e33f64f09345874878c4698578ffdf7ac82a7868723d10aa3825c0102333afe6`
        values = out.decode('UTF-8').split("@")
        sha = values[1].strip().rstrip("'")
        name = values[0].split('/')[-1]
        print(name + ":")
        print("    \"%s\": [\"%s\"]" % (sha, args.tagname))
        cmd = "docker manifest inspect " + mname
        out, err = subprocess.Popen(cmd.split(), stdout=subprocess.PIPE).communicate()
        parsed = json.loads(out)
        for info in parsed["manifests"]:
            img_name = m + "-" + info["platform"]["architecture"] + ":" + args.tagname
            print("%s\n    \"%s\": [\"%s\"]" % (img_name, info["digest"], args.tagname))
        print("\n\n")
    print("\n\n")

if __name__ == '__main__':
    main()

