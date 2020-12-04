#!/bin/bash
# Clean up topologies
./scripts/performance/clean-up-topologies.sh

# N is the depth of the tree. Default to 100 (100 nodes).
N=100

# O is the number of objects per namespace. Default to 1.
O=1

echo "Loading Topology Skewer($N levels, $O objects/node)..."

# Create all namespaces
for ((i=1;i<=N;i++))
do
  echo "Creating namespace tplg-skewer-$i"
  kubectl create ns tplg-skewer-$i
  for ((k=1;k<=O;k++))
  do
    kubectl -n tplg-skewer-$i create role role$k-$i --verb=update --resource=deployments
    kubectl -n tplg-skewer-$i create rolebinding rolebinding$k-$i --role role$k-$i --serviceaccount=tplg-skewer-$i:default
  done
done

# Create skewer topology.
for ((i=2;i<=N;i++))
do
  kubectl hns set tplg-skewer-$i --parent tplg-skewer-$((i-1))
done
