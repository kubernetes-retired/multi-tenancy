#!/bin/bash

bold=$(tput bold)
normal=$(tput sgr0)

# Set up namespace structure
echo "----------------------------------------------------"
echo "${bold}Setting up a 2-level tree with 'a' as the root and 'b' as a subnamespace of 'a'${normal}"
echo "Creating the root 'a'"
kubectl create ns a
sleep 1

echo "${bold}----Creating a subnamespace 'b'${normal}"
echo "Creating subnamespace 'b' for 'a'"
kubectl hns create b -n a

echo "${bold}Here's the outcome of the tree hierarhy:${normal}"
kubectl hns tree a

echo "----------------------------------------------------"
echo "${bold}Test-1:${normal} Remove parent namespace 'a' with 'allowCascadingDelete' unset."
echo "${bold}Operation:${normal} delete 'a' - kubectl delete ns a"
kubectl delete ns a
echo "${bold}Expected:${normal} Forbidden because 'allowCacadingDelete' is not set"

echo "----------------------------------------------------"
echo "${bold}Test-2:${normal} Remove leaf subnamespace without setting 'allowCascadingDelete'."
echo "${bold}Operation:${normal} delete 'b' subnamespace in 'a' - kubectl delete subns b -n a"
kubectl delete subns b -n a
echo "${bold}Expected:${normal} Delete successfully"
echo "${bold}Current hierarhy:${normal} kubectl hns tree a"
kubectl hns tree a

echo "----------------------------------------------------"
echo "${bold}Cleaning up${normal}"
kubectl delete ns a
kubectl delete ns b
