#!/bin/bash

set -e

echo "Listing iptables raw rules on all nodes (all kube-proxy pods)"

for kp in `kubectl get pod -n kube-system -l component=kube-proxy -o custom-columns=:metadata.name --no-headers`; do
  echo ""
  echo "---------------------------------------------------------------------------"
  echo $kp
  echo "---------------------------------------------------------------------------"
  kubectl exec -i -n kube-system $kp -- iptables -t raw --list-rules 
done
