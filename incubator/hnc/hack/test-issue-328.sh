#!/bin/bash

# See https://github.com/kubernetes-sigs/multi-tenancy/issues/328

echo "----------------------------------------------------"
echo "Trying to clean up from last run (this may fail, and that's fine)"
kubectl delete ns parent-328 child-328

# Set up namespace structure
echo "----------------------------------------------------"
echo "Setting up hierarchy with rolebinding that HNC doesn't have permission to copy"
kubectl create ns parent-328
kubectl create ns child-328
sleep 1
kubectl hns set child-328 --parent parent-328
# cluster-admin is the highest-powered ClusterRole and HNC is missing some of
# its permissions, so it cannot propagate it.
kubectl create rolebinding cluster-admin-rb \
  -n parent-328 \
  --clusterrole='cluster-admin' \
  --serviceaccount='parent-328:default'
echo "Waiting 2s..."
sleep 2

echo "----------------------------------------------------"
echo "Tree should show CannotPropagateObject in 'parent-328' and CannotUpdateObject in 'child-328'"
kubectl hns tree parent-328

# Remove the child and verify that the condition is gone
echo "----------------------------------------------------"
echo "Removing the child and verifying that the condition is gone"
kubectl hns set child-328 --root
echo "Waiting 2s..."
sleep 2

echo "----------------------------------------------------"
echo "There should no longer be any conditions in 'parent-328'"
kubectl hns describe parent-328

echo "----------------------------------------------------"
echo "There should no longer be any conditions in 'child-328'"
kubectl hns describe child-328

echo "----------------------------------------------------"
echo "Cleaning up"
kubectl delete ns parent-328 child-328
