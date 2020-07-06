#!/bin/bash
# see https://github.com/kubernetes-sigs/multi-tenancy/issues/824

echo "THIS TEST WILL DELETE CRITICAL PARTS OF HNC. DO NOT RUN UNLESS YOU KNOW WHAT YOU'RE DOING"
echo "You have five seconds to turn back!"
sleep 5


echo "-------------------------------------------------------"
echo "Cleaning up from the last run. This may cause errors"

p=delete-hc-anchor-crd-parent
c=delete-hc-anchor-crd-child

kubectl hns set -a $p
sleep 1
kubectl delete ns $p

# Fail on any error from here on in.
set -e

echo "-------------------------------------------------------"
echo "Creating parent and child"

p=delete-hc-anchor-crd-parent
c=delete-hc-anchor-crd-child

kubectl create ns $p
kubectl hns create $c -n $p

echo "-------------------------------------------------------"
echo "Delete the HC CRD, then wait 1s, then delete the anchor CRD. HNC IS NOW IN A BAD STATE AND MUST BE REINSTALLED"

kubectl delete crd hierarchyconfigurations.hnc.x-k8s.io &
sleep 1
kubectl delete crd subnamespaceanchors.hnc.x-k8s.io &
kubectl delete crd hncconfigurations.hnc.x-k8s.io &

echo "-------------------------------------------------------"
echo "Sleeping for 10s to give HNC the chance to fully delete everything (5s wasn't enough)"

sleep 10

echo "-------------------------------------------------------"
echo "Verify that the HNC CRDs are gone (if nothing's printed, then they are)"

kubectl get crd | grep hnc
