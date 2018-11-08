#!/bin/bash

set -e

# We will intercept the kube-dns service
LOCAL_DNS=`kubectl get services -n kube-system kube-dns -o=jsonpath={.spec.clusterIP}`

if [[ -z "${LOCAL_DNS}" ]]; then
  echo "Unable to find ClusterIP for kube-dns"
  exit 1
fi

echo "Removing iptables raw rules for IP ${LOCAL_DNS} on all nodes (via kube-proxy pods)"

for kp in `kubectl get pod -n kube-system -l component=kube-proxy -o custom-columns=:metadata.name --no-headers`; do
  echo ""
  echo "---------------------------------------------------------------------------"
  echo $kp
  echo "---------------------------------------------------------------------------"
  kubectl exec -i -n kube-system $kp -- iptables -t raw -D PREROUTING -d ${LOCAL_DNS}/32 -p udp -m udp --dport 53 -j NOTRACK || true
  kubectl exec -i -n kube-system $kp -- iptables -t raw -D PREROUTING -d ${LOCAL_DNS}/32 -p tcp -m tcp --dport 53 -j NOTRACK || true
  kubectl exec -i -n kube-system $kp -- iptables -t raw -D OUTPUT -s 10.23.240.10/32 -p udp -m udp --sport 53 -j NOTRACK || true
  kubectl exec -i -n kube-system $kp -- iptables -t raw -D OUTPUT -s 10.23.240.10/32 -p tcp -m tcp --sport 53 -j NOTRACK || true
done
