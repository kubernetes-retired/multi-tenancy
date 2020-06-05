#!/bin/bash

# Fail on any error
set -e

echo "THIS TEST WILL DELETE CRITICAL PARTS OF HNC. DO NOT RUN UNLESS YOU KNOW WHAT YOU'RE DOING"
echo "You have five seconds to turn back!"
sleep 5

echo "-------------------------------------------------------"
echo "Creating parent and child"

kubectl create ns delete-hc-crd-parent
kubectl create ns delete-hc-crd-child
kubectl hns set delete-hc-crd-child --parent delete-hc-crd-parent

echo "-------------------------------------------------------"
echo "Create a rolebinding in parent and validate that it propagates to the child"

kubectl create rolebinding --clusterrole=view --serviceaccount=default:default -n delete-hc-crd-parent foo
sleep 1

echo "-------------------------------------------------------"
echo "The rolebinding should now appear in the child:"

kubectl get -oyaml rolebinding foo -n delete-hc-crd-child

echo "-------------------------------------------------------"
echo "Delete the CRD. HNC IS NOW IN A BAD STATE AND MUST BE REINSTALLED"

kubectl delete customresourcedefinition.apiextensions.k8s.io/hierarchyconfigurations.hnc.x-k8s.io

echo "-------------------------------------------------------"
echo "Sleeping for 5s to give HNC the chance to delete the RB (but it shouldn't)"

sleep 5

echo "-------------------------------------------------------"
echo "Verify that the rolebinding still exists"

kubectl get -oyaml rolebinding foo -n delete-hc-crd-child

echo "Success!!!"
