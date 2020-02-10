#!/bin/bash
echo "Performance Test - Controller Start/Restart Time"

rootname[1]="tplg-wide-0"
rootname[2]="tplg-full-0-0"
rootname[3]="tplg-skewer-1"

for tplg in {1..3}
do
  echo "************Loading the topology ${tplg}************"
  echo "Deleting hnc-controller-manager deployment"
  kubectl -n hnc-system delete deployment hnc-controller-manager 
  echo "Disabling validating webhook"
  kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io/hnc-validating-webhook-configuration
  case $tplg in
  1)
    . ./scripts/performance/load-topology-wide.sh
    ;;
  2)
    . ./scripts/performance/load-topology-full.sh
    ;;
  3)
    . ./scripts/performance/load-topology-skewer.sh
    ;;
  esac
  root=${rootname[tplg]}

  echo "************Starting up the controllers on topology ${tplg}************"
  . ./scripts/performance/start-controllers.sh

  echo "************Restarting the controllers on topology ${tplg}************" 
  echo "Deleting hnc-controller-manager deployment"
  kubectl -n hnc-system delete deployment hnc-controller-manager
  . ./scripts/performance/start-controllers.sh
done
