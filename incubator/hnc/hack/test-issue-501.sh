#!/bin/bash

bold=$(tput bold)
normal=$(tput sgr0)

# Set up namespace structure
echo "----------------------------------------------------"
echo "${bold}Setting up a 3-level tree with 'parent' as the root${normal}"
echo "Creating the root 'parent'"
kubectl create ns parent
sleep 1

echo "${bold}----Creating the 1st branch of owned namespaces----${normal}"
echo "Creating owned namespace 'ochild1' for 'parent'"
kubectl hns create ochild1 -n parent
sleep 1
echo "Creating owned namespace 'ochild1ofochild1' for 'ochild1'"
kubectl hns create ochild1ofochild1 -n ochild1
echo "Creating owned namespace 'ochild2ofochild1' for 'ochild1'"
kubectl hns create ochild2ofochild1 -n ochild1

echo "${bold}----Creating the 2nd branch of owned namespaces----${normal}"
echo "Creating owned namespace 'ochild2' for 'parent'"
kubectl hns create ochild2 -n parent
sleep 1
echo "Creating owned namespace 'ochildofochild2' for 'ochild2'"
kubectl hns create ochildofochild2 -n ochild2

echo "${bold}----Creating the 3rd branch of a mix of unowned and owned namespaces----${normal}"
echo "Creating a namespace 'nochild' (not-owned-child) and set its parent to 'parent'"
kubectl create ns nochild
sleep 1
kubectl hns set nochild --parent parent
echo "Creating owned namespace 'ochildofnochild' for 'nochild'"
kubectl hns create ochildofnochild -n nochild
sleep 1

echo "${bold}Here's the outcome of the tree hierarhy:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-1:${normal} If the owned namespace doesn't allow cascadingDelete and the HNS is missing in the owner namespace, it should have 'HNS_MISSING' condition while its descendants shoudn't have any conditions."
echo "${bold}Operation:${normal} delete 'ochild1' hns in 'parent' - kubectl delete hns ochild1 -n parent"
kubectl delete hns ochild1 -n parent
sleep 1
echo "${bold}Expected:${normal} 'ochild1' namespace is not deleted and should have 'HNS_MISSING' condition; no other conditions."
echo "${bold}Result:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-2:${normal} If the HNS is not missing, it should unset the 'HNS_MISSING' condition in the owned namespace."
echo "${bold}Operation:${normal} recreate the 'ochild1' hns in 'parent' - kubectl hns create ochild1 -n parent"
kubectl hns create ochild1 -n parent
sleep 1
echo "${bold}Expected:${normal} no conditions."
echo "${bold}Result:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-3:${normal} If the owned namespace allows cascadingDelete and the HNS is deleted, it should cascading delete all directly owned namespaces."
echo "${bold}Operation:${normal} 1) allow cascadingDelete in 'ochid1' - kubectl hns set ochild1 --allowCascadingDelete=true"
kubectl hns set ochild1 --allowCascadingDelete=true
echo "2) delete 'ochild1' hns in 'parent' - kubectl delete hns ochild1 -n parent"
kubectl delete hns ochild1 -n parent
echo "Waiting 3s for the namespaces to be deleted..."
sleep 3
echo "${bold}Expected:${normal} 'ochild1', 'ochild1ofochild1', 'ochild2ofochild1' should all be gone"
echo "${bold}Result:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-4:${normal} If the owner namespace allows cascadingDelete and it's deleted, all its directly owned namespaces should be cascading deleted."
echo "${bold}Operation:${normal} 1) allow cascadingDelete in 'parent' - kubectl hns set parent --allowCascadingDelete=true"
kubectl hns set parent --allowCascadingDelete=true
echo "2) delete 'parent' namespace - kubectl delete ns parent"
kubectl delete ns parent
echo "Waiting 3s for the namespaces to be deleted..."
sleep 3
echo "${bold}Expected:${normal} only 'nochild' and 'ochildofnochild' should be left and they should have CRIT_ conditions related to missing 'parent'"
echo "${bold}Result:${normal}"
kubectl hns tree parent
kubectl hns tree ochild2
kubectl hns tree nochild

echo "----------------------------------------------------"
echo "${bold}Cleaning up${normal}"
kubectl hns set nochild --allowCascadingDelete=true
kubectl delete ns nochild
