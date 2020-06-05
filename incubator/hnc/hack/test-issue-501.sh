#!/bin/bash

bold=$(tput bold)
normal=$(tput sgr0)

# Set up namespace structure
echo "----------------------------------------------------"
echo "${bold}Setting up a 3-level tree with 'parent' as the root${normal}"
echo "Creating the root 'parent'"
kubectl create ns parent
sleep 1

echo "${bold}----Creating the 1st branch of subnamespaces----${normal}"
echo "Creating subnamespace 'sub1' for 'parent' without the anchor in the 'parent' namespace"
kubectl create ns sub1
kubectl hns set sub1 --parent parent
kubectl annotate ns sub1 hnc.x-k8s.io/subnamespaceOf=parent
sleep 1
echo "Creating subnamespace 'sub1-sub1' for 'sub1'"
kubectl hns create sub1-sub1 -n sub1
echo "Creating subnamespace 'sub2-sub1' for 'sub1'"
kubectl hns create sub2-sub1 -n sub1

echo "${bold}----Creating the 2nd branch of subnamespaces----${normal}"
echo "Creating subnamespace 'sub2' for 'parent'"
kubectl hns create sub2 -n parent
sleep 1
echo "Creating subnamespace 'sub-sub2' for 'sub2'"
kubectl hns create sub-sub2 -n sub2

echo "${bold}----Creating the 3rd branch of a mix of full and subnamespaces----${normal}"
echo "Creating a namespace 'fullchild' and set its parent to 'parent'"
kubectl create ns fullchild
sleep 1
kubectl hns set fullchild --parent parent
echo "Creating subnamespace 'sub-fullchild' for 'fullchild'"
kubectl hns create sub-fullchild -n fullchild
sleep 1

echo "${bold}Here's the outcome of the tree hierarhy:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-1:${normal} If the subnamespace doesn't allow cascadingDelete and the anchor is missing in the parent namespace, it should have 'SubnamespaceAnchorMissing' condition while its descendants shoudn't have any conditions."
echo "${bold}Expected:${normal} 'sub1' namespace is not deleted and should have 'SubnamespaceAnchorMissing' condition; no other conditions."
echo "${bold}Result:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-2:${normal} If the anchor is readded, it should unset the 'SubnamespaceAnchorMissing' condition in the subnamespace."
echo "${bold}Operation:${normal} recreate the 'sub1' subns in 'parent' - kubectl hns create sub1 -n parent"
kubectl hns create sub1 -n parent
sleep 1
echo "${bold}Expected:${normal} no conditions."
echo "${bold}Result:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-3:${normal} If the subnamespace allows cascadingDelete and the anchor is deleted, it should cascading delete all immediate subnamespaces."
echo "${bold}Operation:${normal} 1) allow cascadingDelete in 'ochid1' - kubectl hns set sub1 --allowCascadingDelete=true"
kubectl hns set sub1 --allowCascadingDelete=true
echo "2) delete 'sub1' subns in 'parent' - kubectl delete subns sub1 -n parent"
kubectl delete subns sub1 -n parent
echo "Waiting 3s for the namespaces to be deleted..."
sleep 3
echo "${bold}Expected:${normal} 'sub1', 'sub1-sub1', 'sub2-sub1' should all be gone"
echo "${bold}Result:${normal}"
kubectl hns tree parent

echo "----------------------------------------------------"
echo "${bold}Test-4:${normal} If the parent namespace allows cascadingDelete and it's deleted, all its subnamespaces should be cascading deleted."
echo "${bold}Operation:${normal} 1) allow cascadingDelete in 'parent' - kubectl hns set parent --allowCascadingDelete=true"
kubectl hns set parent --allowCascadingDelete=true
echo "2) delete 'parent' namespace - kubectl delete ns parent"
kubectl delete ns parent
echo "Waiting 3s for the namespaces to be deleted..."
sleep 3
echo "${bold}Expected:${normal} only 'fullchild' and 'sub-fullchild' should be left and they should have CRIT_ conditions related to missing 'parent'"
echo "${bold}Result:${normal}"
kubectl hns tree parent
kubectl hns tree sub2
kubectl hns tree fullchild

echo "----------------------------------------------------"
echo "${bold}Cleaning up${normal}"
kubectl hns set fullchild --allowCascadingDelete=true
kubectl delete ns fullchild
