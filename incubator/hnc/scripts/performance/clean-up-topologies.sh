#!/bin/bash
echo "Clean up topologies: deleting namespaces with ptlg(toplogy) prefix"
kubectl get ns | awk '/tplg.*/{print $1}' | xargs kubectl delete ns
