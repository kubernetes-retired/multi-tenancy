#!/bin/bash
# Clean up topologies
. ./scripts/performance/clean-up-topologies.sh

# N is the number of children per node(namespace). Default to 20 (421 nodes).
N=20

# O is the number of objects per namespace. Default to 1.
O=1

echo "Loading Topology Full($N children/node, $O objects/node)..."

# Create root namespace
kubectl create ns tplg-full-0-0
for ((k=1;k<=O;k++))
do
  kubectl -n tplg-full-0-0 create configmap configmap$k-0-0 --from-literal key=value
done

# Create all namespaces with tree depth 1
for ((i=1;i<=N;i++))
do
  echo "Creating namespace tplg-full-0-$i"
  kubectl create ns tplg-full-0-$i
  for ((k=1;k<=O;k++))
  do
    kubectl -n tplg-full-0-$i create configmap configmap$k-0-$i --from-literal key=value
  done
  # Create all namespaces with tree depth 2
  for ((j=1;j<=N;j++))
  do
    echo "Creating namespace tplg-full-$i-$j"
    kubectl create ns tplg-full-$i-$j
    for ((k=1;k<=O;k++))
    do
      kubectl -n tplg-full-$i-$j create configmap configmap$k-$i-$j --from-literal key=value
    done
  done
done

# Create full topology.
for ((i=1;i<=N;i++))
do
  kubectl hns set tplg-full-0-$i --parent tplg-full-0-0
  for ((j=1;j<=N;j++))
  do
    kubectl hns set tplg-full-$i-$j --parent tplg-full-0-$i
  done
done
