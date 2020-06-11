#!/bin/bash

bold=$(tput bold)
normal=$(tput sgr0)

# Set up namespace structure
echo "----------------------------------------------------"
echo "${bold}Creating a race condition of two parents with the same non-leaf subns (anchor)"
echo "Creating two root namespaces 'p1' and 'p2'...${normal}"
kubectl create ns p1
kubectl create ns p2
sleep 1

echo "${bold}Disabling webhook to generate the race condition...${normal}"
kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io hnc-validating-webhook-configuration
sleep 1

echo "${bold}Creating subns (anchor) 'sub' in both 'p1' and 'p2'...${normal}"
kubectl hns create sub -n p1
kubectl hns create sub -n p2
sleep 1

echo "${bold}Subnamespace 'sub' should be created and have 'p1' as the 'subnamespaceOf' annoation value:${normal}"
kubectl get ns sub -o yaml

echo "${bold}Creating a 'test-secret' in the subnamespace 'sub'${normal}"
kubectl create secret generic test-secret --from-literal=key=value -n sub

echo "${bold}subns (anchor) 'sub' in 'p2' should have 'status: conflict' because it's a bad anchor:${normal}"
kubectl get subns sub -n p2 -o yaml

echo "${bold}Creating subns (anchor) 'sub-sub' in 'sub' to make 'sub' non-leaf...${normal}"
kubectl hns create sub-sub -n sub
sleep 1

echo "${bold}Enabling webhook again...${normal}"
kubectl apply -f manifests/hnc-manager.yaml

echo "----------------------------------------------------"
echo "${bold}Test-1:${normal} Remove subns (anchor) in the bad parent 'p2'"
echo "${bold}Operation:${normal} - kubectl delete subns sub -n p2"
kubectl delete subns sub -n p2
echo "${bold}Expected:${normal} The bad subns (anchor) is deleted successfully but the 'sub' is not deleted (still contains the 'test-secret'):"
echo "Getting secrets in the 'sub' namespace..."
kubectl get secret -n sub

echo "----------------------------------------------------"
echo "${bold}Test-3:${normal} Remove subns (anchor) in the good parent 'p1'."
echo "${bold}Operation-1:${normal} Setting allowCascadingDelete in 'sub' - kubectl hns set sub -a"
kubectl hns set sub -a
echo "${bold}Operation-2:${normal} - kubectl delete subns sub -n p1"
kubectl delete subns sub -n p1
echo "${bold}Expected:${normal} The subns (anchor) is deleted successfully and the 'sub' and 'sub-sub' are deleted:"
kubectl get ns sub -o yaml
kubectl get ns sub-sub -o yaml

echo "----------------------------------------------------"
echo "${bold}Cleaning up${normal}"
kubectl hns set p1 -a
kubectl delete ns p1
kubectl delete ns p2
