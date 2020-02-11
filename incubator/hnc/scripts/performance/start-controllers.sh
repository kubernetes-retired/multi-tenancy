#!/bin/bash
# Start controllers and output the performance
echo "Creating hnc-controller-manager deployment"
kubectl apply -f manifests/hnc-manager.yaml
ts=$(date -u +"%Y-%m-%dT%T")

echo "Waiting for the controllers to start..."
sleep 10
echo "Looking for logs containing Status:start..."
# Empty the variables.
start=
finish=
# Run until the start is not empty.
timeout=120
until [ ! -z "$start" ]
do
  timeout=$((timeout-1))
  if [ $timeout -lt 1 ]
  then
    echo "Error: no controller activities detected."
    break
  fi
  sleep 1
  start=$(kubectl logs -n hnc-system deployment/hnc-controller-manager -c manager | grep "\"Status\":\"start\"")
done
if [ -z "$start" ]
then
  continue
fi

# Collect all the data in the start log
sec1=`echo ${start} | jq -r '.ts' | grep -Eo ^[0-9]*`

echo "Waiting for the controllers to finish..."
echo "Looking for logs containing Status:finish..."
# Run until the finish is not empty.
timeout=900
until [ ! -z "$finish" ]
do
  timeout=$((timeout-1))
  if [ $timeout -lt 1 ]
  then
    echo "Error: controller activities timed out(600s)."
    break
  fi
  sleep 1
  finish=$(kubectl logs -n hnc-system deployment/hnc-controller-manager -c manager | grep "\"Status\":\"finish\"")
done
if [ -z "$finish" ]
then
  continue
fi

# Collect all the data in the finish log
sec2=`echo ${finish} | jq -r '.ts' | grep -Eo ^[0-9]*`
TotalHierConfigReconciles=`echo ${finish} | jq -r '.TotalHierConfigReconciles' | grep -Eo ^[0-9]*`
TotalObjReconciles=`echo ${finish} | jq -r '.TotalObjReconciles' | grep -Eo ^[0-9]*`
HierConfigWrites=`echo ${finish} | jq -r '.HierConfigWrites' | grep -Eo ^[0-9]*`
NamespaceWrites=`echo ${finish} | jq -r '.NamespaceWrites' | grep -Eo ^[0-9]*`
ObjectWrites=`echo ${finish} | jq -r '.ObjectWrites' | grep -Eo ^[0-9]*`

diffTime=$((sec2-sec1))s

echo "HNC startup time : $ts"
ts=$(date -u +"%Y-%m-%dT%T")
echo "Current time : $ts"
ts=$(date -u -d @${sec1} +"%Y-%m-%dT%T")
echo "Controllers start working time : $ts"
ts=$(date -u -d @${sec2} +"%Y-%m-%dT%T")
echo "Controllers finish working time : $ts"
echo "Controllers working time for HNC startup: $diffTime"
echo "Total HierConfig reconciles: $TotalHierConfigReconciles"
echo "Total Object reconciles: $TotalObjReconciles"
echo "Total HierConfig writes: $HierConfigWrites"
echo "Total Namespace writes: $NamespaceWrites"
echo "Total Object writes: $ObjectWrites"
