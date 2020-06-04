#!/bin/bash

bold=$(tput bold)
normal=$(tput sgr0)

# Set up namespace structure
echo "----------------------------------------------------"
echo "${bold}Setting up a tree with 'a' as the root, 'b' subnamesapce of 'a' and 'c' subnamespace of 'b'${normal}"
echo "Creating the root 'a'"
kubectl create ns a
sleep 1

echo "${bold}----Creating subnamespaces 'b' and 'c'${normal}"
echo "Creating subnamespace 'b' for 'a'"
kubectl hns create b -n a
sleep 1
echo "Creating subnamespace 'c' for 'b'"
kubectl hns create c -n b

echo "${bold}Here's the outcome of the tree hierarhy:${normal}"
kubectl hns tree a

echo "----------------------------------------------------"
echo "${bold}Test-1:${normal} Remove non-leaf subnamespace with 'allowCascadingDelete' unset."
echo "${bold}Operation:${normal} delete 'b' subns in 'a' - kubectl delete subns b -n a"
kubectl delete subns b -n a
echo "${bold}Expected:${normal} Forbidden because 'allowCascadingDelete'flag is not set"

echo "----------------------------------------------------"
echo "${bold}Test-2:${normal} Remove leaf subnamespaces with 'allowCascadingDelete' unset."
echo "${bold}Operation:${normal} delete 'c' subns in 'b' - kubectl delete subns c -n b"
kubectl delete subns c -n b
echo "${bold}Expected:${normal} Delete successfully"
echo "${bold}Operation:${normal} delete 'b' subns in 'a' - kubectl delete subns b -n a"
kubectl delete subns b -n a
echo "${bold}Expected:${normal} Delete successfully"

echo "----------------------------------------------------"
echo "${bold}Cleaning up${normal}"
kubectl delete ns c
kubectl delete ns b
kubectl delete ns a
