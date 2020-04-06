#!/bin/bash

# See https://github.com/kubernetes-sigs/multi-tenancy/issues/605

echo "----------------------------------------------------"
echo "Trying to clean up from last run (this may fail, and that's fine)"
kubectl delete ns parent-605 child-605 grandchild-605 greatgrandchild-605

# Set up namespace structure
echo "----------------------------------------------------"
echo "Setting up hierarchy with rolebinding that HNC doesn't have permission to copy"
kubectl create ns parent-605
kubectl create ns child-605
kubectl create ns grandchild-605
kubectl create ns greatgrandchild-605
sleep 1
kubectl hns set child-605 --parent parent-605
kubectl hns set grandchild-605 --parent child-605
kubectl hns set greatgrandchild-605 --parent grandchild-605
# cluster-admin is the highest-powered ClusterRole and HNC is missing some of
# its permissions, so it cannot propagate it.
kubectl create rolebinding cluster-admin-rb \
  -n parent-605 \
  --clusterrole='cluster-admin' \
  --serviceaccount='parent-605:default'
# We put 30s sleep here because - In issue #605, our current object controller won't reconcile the objects in
# a descendant of a namespace who changes its parent. However, in reality we found that the objects
# (greatgrandchild/cluster-admin-rb in this test) eventually get reconciled, triggered by controller-runtime.
# If sleeping shorter here, controller-runtime will enqueue the object faster since it has an exponential backoff.
# With sleeping 30s, we should see the object gets reconciled after around 8s, triggered by controller-runtime.
# While after fixing #605, we should see it gets reconciled and the obsolete conditions are gone immediately.
# See discussion - https://github.com/kubernetes-sigs/multi-tenancy/pull/617#issuecomment-610588109
echo "Waiting 30s..."
sleep 30

echo "----------------------------------------------------"
echo "Tree should show CannotPropagateObject in 'parent-605' and CannotUpdateObject in 'child-605' and 'grandchild-605'"
kubectl hns tree parent-605

# Remove the grandchild to avoid removing CannotPropagate condition in parent or CannotUpdate condition in child.
# Verify that the conditions in grandchild and greatgrandchild are gone.
echo "----------------------------------------------------"
echo "Removing the grandchild and verifying that the condition is gone in grandchild and greatgrandchild. Parent and child should still have the conditions."
kubectl hns set grandchild-605 --root

# We use 10 because it takes around 9s here before the controller-runtime retry from an exponential backoff
# after the 30s sleep earlier. We should see 'greatgrandchild-605' has conditions for the first 9 attempts before
# #605 is fixed, and should see 'greatgrandchid-605' has no conditions for all 10 attemps after the fix.
N=10
echo "----------------------------------------------------"
echo "There should no longer be any conditions in 'grandchild-605' and 'greatgrandchild-605' in all $N attemps below:"
for ((i=1;i<=N;i++))
do
  echo "----------------------------------------------------"
  echo "Attempt $i/$N - There should no longer be any conditions in 'grandchild-605' and 'greatgrandchild-605'"
  kubectl hns tree grandchild-605
  echo "Waiting 1s.."
  sleep 1
done

echo "----------------------------------------------------"
echo "There should still be CannotPropagate condition in 'parent-605' and CannotUpdate condition in 'child-605'"
kubectl hns tree parent-605

echo "----------------------------------------------------"
echo "Cleaning up"
kubectl delete ns parent-605 child-605 grandchild-605 greatgrandchild-605
