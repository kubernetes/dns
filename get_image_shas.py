# This script pulls the docker images for dns containers and outputs the sha256
# digests for each of the supported arch images.
# These will be used to push the images to k8s.gcr.io.
BUCKET="gcr.io/k8s-staging-dns"
MANIFESTS=["k8s-dns-dnsmasq-nanny", "k8s-dns-kube-dns", "k8s-dns-sidecar", "k8s-dns-node-cache"]
TAG_NAME="1.15.16"
import subprocess
import json

def main():
  for m in MANIFESTS:
    mname = "%s/%s:%s" %(BUCKET, m,TAG_NAME)
    out, err = subprocess.Popen(["docker", "pull", mname], stdout=subprocess.PIPE).communicate()
    if err:
      print "Docker pull hit error - " + err
      return
    maincmd = ["docker", "inspect", "--format='{{index .RepoDigests 0}}'", mname]
    out, err = subprocess.Popen(maincmd,stdout=subprocess.PIPE).communicate()
    values = out.split("@")
    sha = values[1].strip().rstrip("'")
    name = values[0].split("/")[-1]
    print name + ":"
    print "    \"%s\": [\"%s\"]" %(sha, TAG_NAME)
    cmd = "docker manifest inspect " + mname
    out, err = subprocess.Popen(cmd.split(),stdout=subprocess.PIPE).communicate()
    parsed = json.loads(out)
    for info in parsed["manifests"]:
      img_name = m + "-" + info["platform"]["architecture"] + ":" + TAG_NAME
      print "%s\n    \"%s\": [\"%s\"]" %(img_name, info["digest"], TAG_NAME)
    print "\n\n"
  print "\n\n"

if __name__ == '__main__':
  main()
