#!/bin/bash

bold=$(tput bold)
normal=$(tput sgr0)

# Set up namespace structure
echo "----------------------------------------------------"
echo "${bold}Setting up a 2-level tree with 'a' as the root and 'b' as a child of 'a'${normal}"
echo "Creating the root 'a'"
kubectl create ns a
sleep 1
echo "Creating the child namespace 'b' for 'a'"
kubectl create ns b
kubectl hns set b --parent a

echo "${bold}Here's the outcome of the tree hierarhy:${normal}"
kubectl hns tree a

echo "----------------------------------------------------"
echo "${bold}Test-1:${normal} Remove parent namespace 'a'."
echo "${bold}Operation:${normal} delete 'a' - kubectl delete ns a"
kubectl delete ns a
echo "${bold}Expected:${normal} Delete successfully."
echo "${bold}Result:${normal} b should have 'CritParentMissing' condition:"
kubectl hns tree b

echo "----------------------------------------------------"
echo "${bold}Test-2:${normal} Remove the orphan 'b'."
echo "${bold}Operation:${normal} delete 'b' - kubectl delete ns b"
kubectl delete ns b
echo "${bold}Expected:${normal} Delete successfully."

echo "----------------------------------------------------"
echo "${bold}Cleaning up${normal}"
kubectl delete ns a
kubectl delete ns b
