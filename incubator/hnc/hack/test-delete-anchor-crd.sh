#!/bin/bash

# Fail on any error
set -e

echo "THIS TEST WILL DELETE CRITICAL PARTS OF HNC. DO NOT RUN UNLESS YOU KNOW WHAT YOU'RE DOING"
echo "You have five seconds to turn back!"
sleep 5

echo "-------------------------------------------------------"
echo "Creating parent and deletable child"

kubectl create ns delete-crd-parent
kubectl hns create delete-crd-child -n delete-crd-parent
kubectl hns set delete-crd-child --allowCascadingDelete


echo "-------------------------------------------------------"
echo "Delete the CRD. HNC IS NOW IN A BAD STATE AND MUST BE REINSTALLED"

kubectl delete customresourcedefinition.apiextensions.k8s.io/subnamespaceanchors.hnc.x-k8s.io


echo "-------------------------------------------------------"
echo "Verify that the namespace still exists"

kubectl get ns delete-crd-child

echo "Success!!!"
