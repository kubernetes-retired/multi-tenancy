#!/bin/bash

echo "-------------------------------------------------------------"
echo "Setting up simple tree a -> b"

p="issue-772-a"
c="issue-772-b"
kubectl create ns $p
kubectl create ns $c
sleep 1
kubectl hns set $c --parent $p
kubectl hns tree $p


echo "-------------------------------------------------------------"
echo "Creating admin rolebinding object and waiting 2s"

kubectl create rolebinding --clusterrole=admin --serviceaccount=default:default -n $p foo
sleep 2


echo "-------------------------------------------------------------"
echo "Object should exist in the child, and there should be no conditions"

kubectl get rolebinding foo -n $c -oyaml
kubectl hns tree $p

echo "-------------------------------------------------------------"
echo "Cleaning up"
kubectl delete ns $c $p
