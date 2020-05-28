#!/bin/bash

echo "-------------------------------------------------------------"
echo "Setting up simple tree a -> b"

p="issue-771-a"
c="issue-771-b"
kubectl create ns $p
kubectl create ns $c
sleep 1
kubectl hns set $c --parent $p
kubectl hns tree $p


echo "-------------------------------------------------------------"
echo "Creating unpropagatable object; both namespaces should have a condition"

kubectl create rolebinding --clusterrole=cluster-admin --serviceaccount=default:default -n $p foo
sleep 1
kubectl hns tree $p

echo "-------------------------------------------------------------"
echo "Deleting unpropagatable object; all conditions should be cleared"

kubectl delete rolebinding -n $p foo
sleep 1
kubectl hns tree $p

echo "-------------------------------------------------------------"
echo "Cleaning up"
kubectl delete ns $c $p
