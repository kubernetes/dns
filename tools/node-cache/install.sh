#!/bin/bash

set -ex
set -o pipefail

# Create the underlying uncached service
kubectl apply -f kube-dns-uncached.yaml

# We will intercept the kube-dns service
LOCAL_DNS=`kubectl get services -n kube-system kube-dns -o=jsonpath={.spec.clusterIP}`

# And we will forward misses to the uncached service we created above
UPSTREAM_DNS=`kubectl get services -n kube-system kube-dns-uncached -o=jsonpath={.spec.clusterIP}`

# Assume the cluster DNS domain was not changed
DNS_DOMAIN=cluster.local

# nodelocaldns.yaml was sourced from k/k
#
# We should sync it occasionally with
# wget https://raw.githubusercontent.com/kubernetes/kubernetes/master/cluster/addons/dns/nodelocaldns/nodelocaldns.yaml
#
cat nodelocaldns.yaml \
  | sed -e s/addonmanager.kubernetes.io/#addonmanager.kubernetes.io/g \
  | sed -e s@kubernetes.io/cluster-service@#kubernetes.io/cluster-service@g \
  | sed -e 's@k8s-app: kube-dns@k8s-app: nodelocaldns@g' \
  | sed s/__PILLAR__LOCAL__DNS__/${LOCAL_DNS}/g \
  | sed s/__PILLAR__DNS__SERVER__/${UPSTREAM_DNS}/g \
  | sed -e s/__PILLAR__DNS__DOMAIN__/${DNS_DOMAIN}/g \
  | kubectl apply -f -
